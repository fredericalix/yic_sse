package main

import (
	"encoding/json"
	"time"

	"github.com/gofrs/uuid"
)

// Sensor is a sensor message
type Sensor struct {
	AID        uuid.UUID `json:"aid"`
	SID        uuid.UUID `json:"sid"`
	ReceivedAt time.Time `json:"received_at"`

	Data interface{} `json:"data"`
}

// LayoutDB of a city layout stored in DB
type LayoutDB struct {
	AID        uuid.UUID `json:"aid"`
	LID        uuid.UUID `json:"lid"`
	ReceivedAt time.Time `json:"received_at"`

	Data json.RawMessage `json:"data"`
}
