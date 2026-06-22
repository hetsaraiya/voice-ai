// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package observability

import (
	"context"
	"fmt"
)

func (r *recorder) run() {
	defer close(r.done)
	for queuedOperation := range r.operationQueue {
		switch typedOperation := queuedOperation.(type) {
		case addCollectorOperation:
			r.addCollectorWorker(typedOperation.collector)
		case recordOperation:
			preparedObservation := typedOperation.observation
			r.appendReplayObservation(preparedObservation)
			r.broadcastObservation(preparedObservation)
		case closeOperation:
			r.closeCollectorWorkers()
			return
		default:
			r.addError(fmt.Errorf("observability: unsupported recorder operation %T", queuedOperation))
		}
	}
}

type collectorWorker struct {
	key       string
	collector Collector
	queue     chan observation
	done      chan struct{}
	onError   func(error)
}

func newCollectorWorker(key string, collector Collector, onError func(error)) *collectorWorker {
	worker := &collectorWorker{
		key:       key,
		collector: collector,
		queue:     make(chan observation, collectorQueueSize),
		done:      make(chan struct{}),
		onError:   onError,
	}
	go worker.run()
	return worker
}

func (w *collectorWorker) run() {
	defer close(w.done)
	for queuedObservation := range w.queue {
		if err := w.collector.Collect(context.Background(), queuedObservation.scope, queuedObservation.context, queuedObservation.record); err != nil {
			w.onError(err)
		}
	}
	if err := w.collector.Close(context.Background()); err != nil {
		w.onError(err)
	}
}

func (w *collectorWorker) enqueue(observation observation) bool {
	select {
	case w.queue <- observation:
		return true
	default:
		return false
	}
}

func (w *collectorWorker) close() {
	close(w.queue)
	<-w.done
}

func (r *recorder) addCollectorWorker(collector Collector) {
	if collector == nil {
		return
	}
	key := collector.Key()
	if _, ok := r.collectorWorkers[key]; ok {
		return
	}
	worker := newCollectorWorker(key, collector, r.addError)
	r.collectorWorkers[key] = worker
	for _, replayObservation := range r.replayBuffer {
		if !worker.enqueue(replayObservation) {
			r.addError(fmt.Errorf("%w: collector=%s", ErrCollectorBufferFull, key))
			return
		}
	}
}

func (r *recorder) appendReplayObservation(observation observation) {
	if recorderReplayBufferSize <= 0 {
		return
	}
	if len(r.replayBuffer) == recorderReplayBufferSize {
		copy(r.replayBuffer, r.replayBuffer[1:])
		r.replayBuffer[len(r.replayBuffer)-1] = observation
		return
	}
	r.replayBuffer = append(r.replayBuffer, observation)
}

func (r *recorder) broadcastObservation(observation observation) {
	for key, worker := range r.collectorWorkers {
		if !worker.enqueue(observation) {
			r.addError(fmt.Errorf("%w: collector=%s", ErrCollectorBufferFull, key))
		}
	}
}

func (r *recorder) closeCollectorWorkers() {
	for key, worker := range r.collectorWorkers {
		worker.close()
		delete(r.collectorWorkers, key)
	}
}
