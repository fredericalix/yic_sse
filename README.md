# Service of sensors SSE stream

Send sensors data via HTTP SSE protocol from RabbitMQ sensors and ui data for the client.

See the HTTP API documentation in [doc.md](doc.md)

## Compile & run

    go build
    ./sensor-sse

## Exemple config in Environment variables

``` sh
    RABBITMQ_URI="amqp://guest:guest@rabbit:5672"
    AUTH_CHECK_URI="http://auth:1234/auth/check"
    PORT=8080
```
