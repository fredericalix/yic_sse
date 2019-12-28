# Authentification service

## Use the Dockerfile

    docker build

## Compile/Run (manualy)

    cd server
    go build
    ./server

## Configuration via Environment variables

The EXTERNAL_URL is used for setting the link in the token validation email.

``` sh
EXTERNAL_URL="http://localhost:2020"
PORT=1234
RABBITMQ_URI="amqp://guest:guest@rabbit:5672"
POSTGRESQL_URI="postgresql://postgres:yicpass@db:5432/yic_auth?sslmode=disable"

#TLS_CERT=cert.pem
#TLS_KEY=key.pem

#EXPIRATION_VALIDATION="24h"
#EXPIRATION_APP_TOKEN="8760h"
```

## Generate of the swagger doc

Install go-swagger [https://goswagger.io/install.html]() then generate the swagger specification

    cd server
    swagger generate spec -o swagger.json

To quick show the doc

    swagger serve  swagger.json
