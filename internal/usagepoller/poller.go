package usagepoller

import (
	"log/slog"
	"time"

	"github.com/zsprackett/agent-workspace/internal/claudeusage"
	"github.com/zsprackett/agent-workspace/internal/db"
)

type Poller struct {
	store    *db.DB
	interval time.Duration
	stop     chan struct{}
	logger   *slog.Logger
}

func New(store *db.DB, interval time.Duration, logger *slog.Logger) *Poller {
	return &Poller{
		store:    store,
		interval: interval,
		stop:     make(chan struct{}),
		logger:   logger,
	}
}

func (p *Poller) Start() {
	go func() {
		p.poll()
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.poll()
			case <-p.stop:
				return
			}
		}
	}()
}

func (p *Poller) Stop() {
	close(p.stop)
}

func (p *Poller) poll() {
	usage, err := claudeusage.FetchUsage()
	if err != nil {
		p.logger.Debug("usage poll failed", "err", err)
		return
	}

	fiveHourResetsAt := parseResetsAt(usage.FiveHour.ResetsAt)
	sevenDayResetsAt := parseResetsAt(usage.SevenDay.ResetsAt)

	snap := db.UsageSnapshot{
		TsMs:              time.Now().UnixMilli(),
		FiveHourUtil:      usage.FiveHour.Utilization,
		FiveHourResetsAt:  fiveHourResetsAt,
		SevenDayUtil:      usage.SevenDay.Utilization,
		SevenDayResetsAt:  sevenDayResetsAt,
		ExtraEnabled:      usage.ExtraUsage.IsEnabled,
		ExtraMonthlyLimit: usage.ExtraUsage.MonthlyLimit,
		ExtraUsedCredits:  usage.ExtraUsage.UsedCredits,
		ExtraUtilization:  usage.ExtraUsage.Utilization,
	}

	if err := p.store.InsertUsageSnapshot(snap); err != nil {
		p.logger.Debug("usage snapshot insert failed", "err", err)
	}
}

// parseResetsAt converts an RFC3339 timestamp string to Unix milliseconds.
// Returns 0 on parse failure.
func parseResetsAt(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try RFC3339Nano as fallback
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return 0
		}
	}
	return t.UnixMilli()
}

