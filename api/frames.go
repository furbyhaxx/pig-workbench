package api

// FrameDeliveryChanged is the frames.deliveryChanged event payload, sent to the producer's registration.
type FrameDeliveryChanged struct {
	FrameID  string `json:"frameId"`
	Source   string `json:"source"`
	EventKey string `json:"eventKey"`
	// State is the frame's new delivery state; "delivered" once a provider request carried it.
	State string `json:"state"`
}
