package watch

import (
	"os"
	"sync"
	"sort"
	"path/filepath"
	"time"

	"github.com/rjeczalik/notify"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/ignite/pkg/storage"
	"github.com/weaveworks/ignite/pkg/storage/watch/update"
	"github.com/weaveworks/ignite/pkg/util"
)

const eventBuffer = 4096 // How many events and updates we can buffer before watching is interrupted
var excludeDirs = []string{".git"}
var listenEvents = []notify.Event{notify.InCreate, notify.InDelete, notify.InDeleteSelf, notify.InCloseWrite}

var eventMap = map[notify.Event]update.Event{
	notify.InCreate:     update.EventCreate,
	notify.InDelete:     update.EventDelete,
	notify.InCloseWrite: update.EventModify,
}

// combinedEventsMap maps multiple, sorted events into one single event that should be dispatched
var combinedEventsMap = map[string]update.Event{
	// DELETE+CREATE+MODIFY => MODIFY
	update.Events{update.EventCreate, update.EventDelete, update.EventModify}.String(): update.EventModify,
	// CREATE+MODIFY => CREATE
	update.Events{update.EventCreate, update.EventModify}.String(): update.EventCreate,
}

type eventStream chan notify.EventInfo
type UpdateStream chan *update.FileUpdate
type watches []string

// watcher recursively monitors changes in files in the given directory
// and sends out events based on their state changes. Only files conforming
// to validSuffix are monitored. The watcher can be suspended for a single
// event at a time to eliminate updates by WatchStorage causing a loop.
type watcher struct {
	dir          string
	events       eventStream
	updates      UpdateStream
	watches      watches
	suspendEvent update.Event
	monitor      *util.Monitor
	dispatcher      *util.Monitor
	// the batcher is used for properly sending many concurrent inotify events
	// as a group, after a specified timeout. This fixes the issue of one single
	// file operation being registered as many different inotify events
	batcher *Batcher
	// eventCache is used together with the batcher. It's a sync.Map for supporting
	// concurrent operations. In reality, the map is of type map[string]update.Events,
	// holding information around all the events that have been registered for the same file
	eventCache *sync.Map
}

func (w *watcher) addWatch(path string) (err error) {
	log.Tracef("Watcher: adding watch for %q", path)
	if err = notify.Watch(path, w.events, listenEvents...); err == nil {
		w.watches = append(w.watches, path)
	}

	return
}

func (w *watcher) hasWatch(path string) bool {
	for _, watch := range w.watches {
		if watch == path {
			log.Tracef("Watcher: watch found for %q", path)
			return true
		}
	}

	log.Tracef("Watcher: no watch found for %q", path)
	return false
}

func (w *watcher) clear() {
	log.Tracef("Watcher: clearing all watches")
	notify.Stop(w.events)
	w.watches = w.watches[:0]
}

// newWatcher returns a list of files in the watched directory in
// addition to the generated watcher, it can be used to populate
// MappedRawStorage fileMappings
func newWatcher(dir string) (w *watcher, files []string, err error) {
	eventCache := &sync.Map{}
	// the batcher waits one second after the last event before dispatching
	// the inotify events grouped
	dispatchDuration := 1 * time.Second
	w = &watcher{
		dir:     dir,
		events:  make(eventStream, eventBuffer),
		updates: make(UpdateStream, eventBuffer),
		eventCache: eventCache,
		batcher: NewBatcher(eventCache, dispatchDuration),
	}

	if err = w.start(&files); err != nil {
		notify.Stop(w.events)
	} else {
		w.monitor = util.RunMonitor(w.monitorFunc)
	}

	w.dispatcher = util.RunMonitor(w.dispatchFunc)

	return
}

// start discovers all subdirectories and adds paths to
// notify before starting the monitoring goroutine
func (w *watcher) start(files *[]string) error {
	return filepath.Walk(w.dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				for _, dir := range excludeDirs {
					if info.Name() == dir {
						return filepath.SkipDir // Skip excluded directories
					}
				}

				return w.addWatch(path)
			}

			if files != nil {
				// Only include files with a valid suffix
				if validSuffix(info.Name()) {
					*files = append(*files, path)
				}
			}

			return nil
		})
}

func (w *watcher) monitorFunc() {
	log.Debug("Watcher: monitoring thread started")
	defer log.Debug("Watcher: monitoring thread stopped")
	defer close(w.updates) // Close the update stream after the watcher has stopped

	for {
		event, ok := <-w.events
		if !ok {
			return
		}

		// If there's a pending timer that's unfired, cancel it as we got this new event within the timeout duration
		w.batcher.CancelUnfiredTimer()

		updateEvent := convertEvent(event.Event())
		if updateEvent == w.suspendEvent {
			w.suspendEvent = 0
			log.Debugf("Watcher: skipping suspended event %s for path: %s", updateEvent, event.Path())
			continue // Skip the suspended event
		}

		// Get any events registered for the specific file, and append the specified event
		var eventList update.Events
		val, ok := w.eventCache.Load(event.Path())
		if ok {
			eventList = val.(update.Events)
		}
		eventList = append(eventList, updateEvent)

		// Register the event in the map
		w.eventCache.Store(event.Path(), eventList)
		log.Debugf("Watcher: Registered inotify events %v for path %s", eventList, event.Path())

		// Dispatch all the currently registered events after the configured timeout
		w.batcher.DispatchAfterTimeout()
	}
}

func (w *watcher) dispatchFunc() {
	log.Debug("Watcher: dispatch thread started")
	defer log.Debug("Watcher: dispatch thread stopped")

	for {
		// wait until we have a batch dispatched to us
		w.batcher.ProcessBatch(func(key, val interface{}) bool {
			filePath := key.(string)
			events := val.(update.Events)
			event := events[0]
			// If there are multiple events, choose the correct one based on the combination
			if len(events) > 1 {
				sort.Sort(events)
				eventsStr := events.String()
				var ok bool
				event, ok = combinedEventsMap[eventsStr]
				if !ok {
					log.Errorf("Watcher: no known event combination for %v", events)
					return true
				}
			}
			// dispatch this event to w.updates
			w.handleEvent(filePath, event)
			// continue traversing the map
			return true
		})
		log.Debug("Watcher: dispatched events batch and reset the events cache")
	}
}


func (w *watcher) handleEvent(filePath string, event update.Event) {
	switch event {
	case update.EventCreate:
		fi, err := os.Stat(filePath)
		if err != nil {
			log.Errorf("Watcher: failed to stat %q: %v", filePath, err)
			return
		}

		if fi.IsDir() {
			if err := w.addWatch(filePath); err != nil {
				log.Errorf("Watcher: failed to add %q: %v", filePath, err)
			}

			return
		}

		fallthrough
	case update.EventDelete, update.EventModify:
		if event == update.EventDelete && w.hasWatch(filePath) {
			w.clear()
			if err := w.start(nil); err != nil {
				log.Errorf("Watcher: Failed to re-initialize watches for %q", w.dir)
			}

			return
		}

		// only care about valid files
		if !validSuffix(filePath) {
			return
		}

		log.Debugf("Watcher: Sending update: %s -> %q", event, filePath)
		w.updates <- &update.FileUpdate{
			Event: event,
			Path:  filePath,
		}
	}
}

// TODO: This watcher doesn't handle multiple operations on the same file well
// DELETE+CREATE+MODIFY => MODIFY
// CREATE+MODIFY => CREATE
// Fix this by caching the operations on the same file, and one second after all operations
// have been "written"; go through the changes and interpret the combinations of events properly
// This maybe will allow us to remove the "suspend" functionality? I don't know yet

func (w *watcher) close() {
	notify.Stop(w.events)
	w.batcher.Close()
	close(w.events) // Close the event stream
	w.monitor.Wait()
	w.dispatcher.Wait()
}

// This enables a one-time suspend of the given event,
// the watcher will skip the given event once
func (w *watcher) suspend(updateEvent update.Event) {
	w.suspendEvent = updateEvent
}

func convertEvent(event notify.Event) update.Event {
	if updateEvent, ok := eventMap[event]; ok {
		return updateEvent
	}

	return 0
}

// validSuffix is used to filter out all unsupported
// files based on the extensions in storage.Formats
func validSuffix(path string) bool {
	for suffix := range storage.Formats {
		if filepath.Ext(path) == suffix {
			return true
		}
	}

	return false
}
