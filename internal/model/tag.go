package model

// Tag is a key-value metadata pair attached to a network or subnet.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
