package lockstep

const (
	MatchCount               = 1
	UpdateIntervalMillis     = 50
	FrameCountPerSecond      = 1000 / UpdateIntervalMillis
	SaveLSWorldFrameCount    = 1200
	MaxPredictionFrameWindow = 5
	AdjustTimeThreshold      = 3

	SessionTimeoutMillis = 40000
)
