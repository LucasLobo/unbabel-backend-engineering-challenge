package application

import (
	"time"

	"github.com/lucaslobo/aggregator-cli/internal/core/domain"
	"github.com/lucaslobo/aggregator-cli/internal/core/outboundprt"
)

type Application struct {
	storer outboundprt.MovingAverageStorer
	sw     *slidingWindow
}

func New(windowSize int, storer outboundprt.MovingAverageStorer) *Application {
	return &Application{
		storer: storer,
		sw: &slidingWindow{
			windowSize: windowSize,
			buckets:    map[time.Time]state{},
		},
	}
}

type state struct {
	count    int
	duration int
}

type slidingWindow struct {
	windowSize int
	buckets    map[time.Time]state
	state      state

	start time.Time
	head  time.Time
	tail  time.Time
}

// ProcessEvent calculates the moving average for all time-buckets since the last event. If this is the first event
// it initializes the time-buckets. The moving-average is calculated based on the windowSize provided in the Init method
func (a *Application) ProcessEvent(event domain.TranslationDelivered) error {
	bucket := event.Timestamp.Truncate(time.Minute).Add(time.Minute)

	sw := a.sw
	// we must initialize the values when the first event is processed
	if sw.start.IsZero() {
		start := bucket.Add(-time.Minute)
		sw.start = start
		sw.head = start
		sw.tail = start
	}

	// We must iterate X times until we get to the current event time bucket
	for beforeOrEqual(sw.head, bucket) {

		sw.buckets[sw.head] = state{}

		// when we are at the time bucket of the current event, we add it to the state
		if sw.head.Equal(bucket) {
			sw.state.count += 1
			sw.state.duration += event.Duration
			sw.buckets[sw.head] = state{
				duration: event.Duration,
				count:    1,
			}
		}

		// when we exceed the current window size, we must remove the last item and advance the tail
		if len(sw.buckets) > sw.windowSize {
			sw.state.count -= sw.buckets[sw.tail].count
			sw.state.duration -= sw.buckets[sw.tail].duration

			delete(sw.buckets, sw.tail)
			sw.tail = sw.tail.Add(time.Minute)
		}

		// once we're done, we calculate the average for the current position
		average := float32(0)
		if sw.state.count != 0 {
			// let's not divide by 0 ;)
			average = float32(sw.state.duration) / float32(sw.state.count)
		}

		adt := domain.AverageDeliveryTime{
			Date:                domain.Time{Time: sw.head},
			AverageDeliveryTime: average,
		}

		err := a.storer.StoreMovingAverage(adt)
		if err != nil {
			return err
		}

		// at the end we must advance the head to keep going
		sw.head = sw.head.Add(time.Minute)
	}
	return nil
}

func beforeOrEqual(a, b time.Time) bool {
	return a.Before(b) || a.Equal(b)
}
