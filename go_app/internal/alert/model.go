package alert

// Warning represents the structure of a weather alert warning.
type Warning struct {
	Type         string   `json:"type"`
	Event        string   `json:"event"`
	Areas        []string `json:"areas"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Office       string   `json:"office"`
	Time         string   `json:"time"`
	HailSize     any      `json:"hail_size"`
	Status       string   `json:"status"`
}
