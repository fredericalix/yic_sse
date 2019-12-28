package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "net/http/pprof"

	"github.com/gofrs/uuid"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	echolog "github.com/labstack/gommon/log"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"

	"github.com/fredericalix/yic_sse/sse"
)

func failOnError(err error, msg string) {
	if err != nil {
		msg := fmt.Errorf("%s: %s", msg, err)
		log.Fatal(msg)
	}
}

func main() {
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")

	configFile := flag.String("config", "./config.toml", "path of the config file")
	flag.Parse()
	viper.SetConfigFile(*configFile)
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		log.Printf("cannot read config file: %v\nUse env instead\n", err)
	}

	if viper.GetString("RABBITMQ_URI") == "" {
		panic("missing RABBITMQ_URI")
	}
	h := newHandler(viper.GetString("RABBITMQ_URI"))

	// init http server
	e := echo.New()
	e.Use(middleware.Logger())
	e.Logger.SetLevel(echolog.INFO)

	a := e.Group("/sse")
	a.GET("", h.getSSE, auth.Middleware(auth.NewValidHTTP(viper.GetString("AUTH_CHECK_URI")), auth.Roles{"sensor": "r", "ui": "r"}))
	a.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux), middleware.Rewrite(map[string]string{"/sse/*": "/$1"}))

	// start the server
	host := ":" + viper.GetString("PORT")
	tlscert := viper.GetString("TLS_CERT")
	tlskey := viper.GetString("TLS_KEY")
	if tlscert == "" || tlskey == "" {
		e.Logger.Error("No cert or key provided. Start server using HTTP instead of HTTPS !")
		e.Logger.Fatal(e.Start(host))
	}
	e.Logger.Fatal(e.StartTLS(host, tlscert, tlskey))
}

type handler struct {
	amqpURI  string
	maxRetry int

	conn       *amqp.Connection
	closeError chan *amqp.Error
	lock       sync.RWMutex
}

// Inspired by https://gist.github.com/firstrow/412f5b2444db10bfc532ec0e87d6ea1e
func newHandler(amqpURI string) *handler {
	h := &handler{
		amqpURI:  amqpURI,
		maxRetry: 4,
	}
	closeError := make(chan *amqp.Error)

	go func() {
		for {
			rabbitErr := <-closeError
			if rabbitErr != nil {
				log.Println(rabbitErr)
				log.Printf("Reconnecting to %s\n", amqpURI)

				conn := h.connectToRabbitMQ(amqpURI)
				h.lock.Lock()
				h.conn = conn
				closeError = make(chan *amqp.Error)
				conn.NotifyClose(closeError)
				h.lock.Unlock()
			}
		}
	}()

	// first connection
	log.Printf("Connecting to %s\n", amqpURI)
	h.conn = h.connectToRabbitMQ(amqpURI)
	h.conn.NotifyClose(closeError)

	return h
}

func (h *handler) connectToRabbitMQ(uri string) *amqp.Connection {
	var err error
	for retry := 0; retry < h.maxRetry; retry++ {
		var conn *amqp.Connection
		conn, err = amqp.Dial(uri)
		if err == nil {
			log.Printf("Connected to RabbitMQ via %v", conn.LocalAddr())
			return conn
		}

		log.Printf("Failed to connect to RabbitMQ retry %d/%d: %v", retry, h.maxRetry, err)
		time.Sleep(4 * time.Second)
	}
	log.Fatalf("Reach max conection retry to RabbitMQ of %d... Exit", h.maxRetry)
	return nil
}

func (h *handler) channel() (*amqp.Channel, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	return h.conn.Channel()
}

func (h *handler) getSSE(c echo.Context) error {
	corrID := randID()

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	// check Auth
	aid := c.Get("account").(auth.Account).ID

	c.Logger().Infof("sse connection from aid=%v CORRID=%v", aid, corrID)

	close := c.Response().CloseNotify()

	ch, err := h.channel()
	failOnError(err, "getSSE new channel")
	defer ch.Close()

	chclosed := ch.NotifyClose(make(chan *amqp.Error))

	sch, err := newAMQPSensors(ch, aid)
	if err != nil {
		c.Logger().Errorf("GetSSE newAMQPSensors: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Interal Server Error"})
	}

	latestSensors, err := findLatestSensors(ctx, ch, aid, corrID)
	if err != nil {
		c.Logger().Errorf("GetSSE findLatestSensors: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Interal Server Error"})
	}

	lch, err := newAMQPUILayout(ch, aid)
	if err != nil {
		c.Logger().Errorf("GetSSE newAMQPUILayout: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Interal Server Error"})
	}
	latestLayout, err := findLatestLayout(ctx, ch, aid, corrID)
	if err != nil {
		c.Logger().Errorf("GetSSE findLatestSensors: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Interal Server Error"})
	}

	// set SSE response
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().WriteHeader(http.StatusOK)

	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	for { // infinite loop
		select {
		case <-ctx.Done():
			return nil
		case <-close: // sse connection closed by client
			return nil

		case err, ok := <-chclosed:
			return fmt.Errorf("Channel closed: ok: %v err: %v", ok, err)

		case <-heartbeat.C:
			sse.SendEvent(c.Response(), "heartbeat", "")

		case sensors := <-latestSensors: // get latest sensors
			for _, s := range sensors {
				b, _ := json.Marshal(s.Data)
				sse.AddEvent(c.Response(), "sensors", string(b))
			}
			sse.Send(c.Response())
			latestSensors = nil
			c.Logger().Infof("Sent to sensors %v sensors COORID:%v", len(sensors), corrID)

		case sensor := <-sch: // new sensors message
			sse.SendEvent(c.Response(), "sensors", string(sensor.Body))

		case layouts := <-latestLayout: // get latest ui layout
			for _, l := range layouts {
				b, _ := json.Marshal(l.Data)
				sse.AddEvent(c.Response(), "ui.layout", string(b))
			}
			sse.Send(c.Response())
			latestLayout = nil
			c.Logger().Infof("Sent to ui.layouts %v layouts COORID:%v", len(layouts), corrID)

		case layout := <-lch: // new ui.layout message
			rkey := strings.Split(layout.RoutingKey, ".")
			if rkey[len(rkey)-1] == "delete" {
				sse.SendEvent(c.Response(), "ui.layout.delete", rkey[1])
				c.Logger().Infof("Sent to ui.layout.delete %v COORID:%v", layout.RoutingKey, corrID)
			} else { // update/create
				sse.SendEvent(c.Response(), "ui.layout", string(layout.Body))
				c.Logger().Infof("Sent to ui.layout %v COORID:%v", layout.RoutingKey, corrID)
			}
		}
	}
}

func randID() string {
	return base64.RawURLEncoding.EncodeToString(uuid.Must(uuid.NewV4()).Bytes())
}

func newAMQPSensors(ch *amqp.Channel, aid uuid.UUID) (<-chan amqp.Delivery, error) {
	err := ch.ExchangeDeclare(
		"sensors", // name
		"topic",   // type
		true,      // durable
		false,     // auto-deleted
		false,     // internal
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare an exchange: %v", err)
	}

	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when usused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare an queue: %v", err)
	}

	// Get every routing key
	err = ch.QueueBind(
		q.Name,            // queue name
		aid.String()+".#", // routing key
		"sensors",         // exchange
		false,             // no wait
		nil,               // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to bind a queue: %v", err)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto ack
		false,  // exclusive
		false,  // no local
		false,  // no wait
		nil,    // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register a consume: %v", err)
	}
	return msgs, nil
}

func findLatestSensors(ctx context.Context, ch *amqp.Channel, aid uuid.UUID, corrID string) (<-chan []Sensor, error) {
	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when usused
		true,  // exclusive
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		return nil, err
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, err
	}

	request := struct {
		AID uuid.UUID `json:"aid"`
	}{aid}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	err = ch.Publish(
		"",                   // exchange
		"rpc_sensors_latest", // routing key
		false,                // mandatory
		false,                // immediate
		amqp.Publishing{
			ContentType:   "text/plain",
			CorrelationId: corrID,
			ReplyTo:       q.Name,
			Body:          body,
		},
	)
	if err != nil {
		return nil, err
	}

	latests := make(chan []Sensor)

	go func() {
		defer func() {
			close(latests)
			ch.QueueDelete(q.Name, false, false, false)
		}()

		for {
			select {
			case <-ctx.Done():
				return

			case d := <-msgs:
				if corrID == d.CorrelationId {
					var s []Sensor
					err := json.Unmarshal(d.Body, &s)
					if err != nil {
						log.Println(err)
						return
					}
					latests <- s
					d.Ack(false)
					return
				}
			}
		}
	}()

	return latests, nil
}

func newAMQPUILayout(ch *amqp.Channel, aid uuid.UUID) (<-chan amqp.Delivery, error) {
	err := ch.ExchangeDeclare(
		"ui.layout", // name
		"topic",     // type
		true,        // durable
		false,       // auto-deleted
		false,       // internal
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare an exchange: %v", err)
	}

	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when usused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare an queue: %v", err)
	}

	// Get every routing key
	err = ch.QueueBind(
		q.Name,            // queue name
		aid.String()+".#", // routing key
		"ui.layout",       // exchange
		false,             // no wait
		nil,               // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to bind a queue: %v", err)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto ack
		false,  // exclusive
		false,  // no local
		false,  // no wait
		nil,    // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register a consume: %v", err)
	}
	return msgs, nil
}

func findLatestLayout(ctx context.Context, ch *amqp.Channel, aid uuid.UUID, corrID string) (<-chan []LayoutDB, error) {
	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		true,  // delete when usused
		true,  // exclusive
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		return nil, err
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, err
	}

	request := struct {
		AID uuid.UUID `json:"aid"`
	}{aid}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	err = ch.Publish(
		"",              // exchange
		"rpc_ui_latest", // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType:   "text/plain",
			CorrelationId: corrID,
			ReplyTo:       q.Name,
			Body:          body,
		},
	)
	if err != nil {
		return nil, err
	}

	latests := make(chan []LayoutDB)

	go func() {
		defer func() {
			close(latests)
			ch.QueueDelete(q.Name, false, false, false)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case d := <-msgs:
				if corrID == d.CorrelationId {
					var l []LayoutDB
					err := json.Unmarshal(d.Body, &l)
					if err != nil {
						log.Println(err)
						return
					}
					latests <- l
					return
				}
			}
		}
	}()

	return latests, nil
}
