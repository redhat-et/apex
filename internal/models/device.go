package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Device is a unique, end-user device.
type Device struct {
	Base
	UserID    string      `json:"user_id" example:"694aa002-5d19-495e-980b-3d8fd508ea10"`
	PublicKey string      `gorm:"uniqueIndex" json:"public_key"`
	Peers     []*Peer     `json:"-"`
	PeerList  []uuid.UUID `gorm:"-" json:"peers" example:"97d5214a-8c51-4772-b492-53de034740c5"`
	Hostname  string      `json:"hostname" example:"myhost"`
}

func (d *Device) BeforeCreate(tx *gorm.DB) error {
	if d.Peers == nil {
		d.Peers = make([]*Peer, 0)
	}
	if d.PeerList == nil {
		d.PeerList = make([]uuid.UUID, 0)
	}
	return d.Base.BeforeCreate(tx)
}

// AddDevice is the information needed to add a new Device.
type AddDevice struct {
	PublicKey string `json:"public_key" example:"rZlVfefGshRxO+r9ethv2pODimKlUeP/bO/S47K3WUk="`
	Hostname  string `json:"hostname" example:"myhost"`
}
