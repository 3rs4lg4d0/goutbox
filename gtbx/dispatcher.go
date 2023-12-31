package gtbx

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type dispatcher struct {
	id         uuid.UUID
	settings   Settings
	logger     Logger
	emitter    Emitter
	repository Repository
	successCtr Counter
	errorCtr   Counter
}

// launchDispatcher starts a subscription loop to attempt the registration of a new dispatcher
// within the 'outbox_dispatcher_subscription'. Only subscribed dispatchers can deliver
// outbox entries to the configured emitter. The function also ensures the consistent updating
// of the "alive_at" column to avoid losing the dispatcher subscription.
func (d *dispatcher) launchDispatcher() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	subscribed := false
	for ; true; <-ticker.C {
		if !subscribed {
			if success, subscription, err := d.repository.SubscribeDispatcher(d.id, d.settings.MaxDispatchers); success {
				d.logger.Debug(fmt.Sprintf("subscription '%d' assigned to dispatcher '%s'", subscription, d.id))
				go d.executeDispatcherLoop()
				subscribed = true
			} else if err != nil {
				d.logger.Error(fmt.Sprintf("trying to subscribe dispatcher '%s'", d.id), err)
			}
		} else {
			updated, err := d.repository.UpdateSubscription(d.id)
			if err != nil {
				d.logger.Error("updating subscription", err)
			} else if !updated {
				d.logger.Error("subscription not updated", errors.New("stolen subscription!"))
				subscribed = false
			}
		}
	}
}

// executeDispatcherLoop implements the main dispatcher loop.
func (d *dispatcher) executeDispatcherLoop() {
	ticker := time.NewTicker(d.settings.PollingInterval)
	for ; true; <-ticker.C {
		if acquired, err := d.acquireOutboxLock(); acquired {
			d.processOutbox()
			err := d.releaseOutboxLock()
			if err != nil {
				d.logger.Error("releasing the outbox lock", err)
			}
		} else if err != nil {
			d.logger.Error("unable to get the lock", err)
		}
	}
}

// acquireOutboxLock acquires a lock on 'outbox' table to ensure that only one
// dispatcher can be active at a time to process outbox deliveries.
func (d *dispatcher) acquireOutboxLock() (bool, error) {
	return d.repository.AcquireLock(d.id)
}

// releaseOutboxLock releases the acquired lock in acquireOutboxLock().
func (d *dispatcher) releaseOutboxLock() error {
	return d.repository.ReleaseLock(d.id)
}

// processOutbox scans the 'outbox' table within the limits defined by Settings.MaxEventsPerInterval
// and delivers the outbox entries in batches (defined by Settings.defaultMaxEventsPerBatch).
func (d *dispatcher) processOutbox() {
	var success []uuid.UUID
	var totalProcessed int
	var totalErr int
	var deliveryChan = make(chan *DeliveryReport, d.settings.MaxEventsPerBatch)
	var wg sync.WaitGroup

	d.logger.Debug("processing outbox messages")

	go func() {
		for dr := range deliveryChan {
			if dr.Error != nil {
				d.logger.Error("delivery problem", dr.Error)
				totalErr++
				d.errorCtr.Inc(1)
			} else {
				d.logger.Debug(dr.Details)
				success = append(success, dr.Record.Id)
				d.successCtr.Inc(1)
			}
			totalProcessed++
			wg.Done()
		}
		d.logger.Debug("the goroutine for delivery reports has finished")
	}()

	err := d.repository.FindInBatches(d.settings.MaxEventsPerBatch, d.settings.MaxEventsPerInterval, func(batch []*OutboxRecord) error {
		d.logger.Debug(fmt.Sprintf("Sending %d messages to kafka", len(batch)))
		for _, o := range batch {
			err := d.emitter.Emit(o, deliveryChan)
			if err != nil {
				d.logger.Error("when producing a message", err)
				// if any error happen sending the message we don't need to retry here,
				// the message will remain in the outbox table and will be sent in the
				// next outbox processing.
			} else {
				wg.Add(1)
			}
		}
		return nil
	})

	if err != nil {
		d.logger.Error("when trying to get outbox rows in batches", err)
	}

	// Wait until we get all the delivery reports from kafka client.
	wg.Wait()

	// We can safely close the channel because this is a dedicated channel only to
	// receive as many delivery reports as many messages are sent.
	close(deliveryChan)
	d.logger.Info(fmt.Sprintf("%d messages were successfully delivered (with %d failed) from a total of %d processed from outbox", len(success), totalErr, totalProcessed))

	if len(success) > 0 {
		d.logger.Debug(fmt.Sprintf("Deleting %d elements from outbox", len(success)))
		err := d.repository.DeleteInBatches(d.settings.MaxEventsPerBatch, success)
		if err != nil {
			d.logger.Error("when deleting sent outbox records in batches", err)
		}
	}
}
