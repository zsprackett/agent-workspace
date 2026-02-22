package claudeusage

type UsageResponse struct {
	FiveHour   WindowUsage `json:"five_hour"`
	SevenDay   WindowUsage `json:"seven_day"`
	ExtraUsage ExtraUsage  `json:"extra_usage"`
	Error      string      `json:"error,omitempty"`
}

type WindowUsage struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type ExtraUsage struct {
	IsEnabled    bool    `json:"is_enabled"`
	MonthlyLimit float64 `json:"monthly_limit"` // cents
	UsedCredits  float64 `json:"used_credits"`  // cents
	Utilization  float64 `json:"utilization"`
}
