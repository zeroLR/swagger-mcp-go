package registry

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"github.com/zeroLR/swagger-mcp-go/internal/models"
)

// Registry manages OpenAPI specifications with TTL-based caching
type Registry struct {
	specs   map[string]*models.SpecInfo
	mutex   sync.RWMutex
	logger  *zap.Logger
	events  chan SpecEvent
}

// SpecEvent represents a specification change event
type SpecEvent struct {
	Type        SpecEventType `json:"type"`
	ServiceName string        `json:"serviceName"`
	SpecInfo    *models.SpecInfo `json:"specInfo,omitempty"`
	Error       string        `json:"error,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
}

// SpecEventType represents the type of spec event
type SpecEventType string

const (
	SpecEventAdded   SpecEventType = "spec.added"
	SpecEventUpdated SpecEventType = "spec.updated"
	SpecEventRemoved SpecEventType = "spec.removed"
	SpecEventError   SpecEventType = "spec.error"
)

// New creates a new registry instance
func New(logger *zap.Logger) *Registry {
	return &Registry{
		specs:  make(map[string]*models.SpecInfo),
		logger: logger,
		events: make(chan SpecEvent, 100),
	}
}

// Add registers a new OpenAPI specification
func (r *Registry) Add(specInfo *models.SpecInfo) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	existing, exists := r.specs[specInfo.ServiceName]
	r.specs[specInfo.ServiceName] = specInfo

	eventType := SpecEventAdded
	if exists {
		eventType = SpecEventUpdated
		r.logger.Info("Updated spec for service",
			zap.String("serviceName", specInfo.ServiceName),
			zap.String("url", specInfo.URL),
			zap.Time("previousFetch", existing.FetchedAt))
	} else {
		r.logger.Info("Added new spec for service",
			zap.String("serviceName", specInfo.ServiceName),
			zap.String("url", specInfo.URL))
	}

	// Emit event
	r.emitEvent(SpecEvent{
		Type:        eventType,
		ServiceName: specInfo.ServiceName,
		SpecInfo:    specInfo,
		Timestamp:   time.Now(),
	})

	return nil
}

// Get retrieves a specification by service name
func (r *Registry) Get(serviceName string) (*models.SpecInfo, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	spec, exists := r.specs[serviceName]
	if !exists {
		return nil, false
	}

	// Check if spec is expired
	if r.isExpired(spec) {
		r.logger.Debug("Spec expired",
			zap.String("serviceName", serviceName),
			zap.Time("fetchedAt", spec.FetchedAt),
			zap.Duration("ttl", spec.TTL))
		return spec, false // Return spec but indicate it needs refresh
	}

	return spec, true
}

// Remove removes a specification from the registry
func (r *Registry) Remove(serviceName string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.specs[serviceName]; !exists {
		return false
	}

	delete(r.specs, serviceName)

	r.logger.Info("Removed spec for service", zap.String("serviceName", serviceName))

	// Emit event
	r.emitEvent(SpecEvent{
		Type:        SpecEventRemoved,
		ServiceName: serviceName,
		Timestamp:   time.Now(),
	})

	return true
}

// List returns all registered specifications
func (r *Registry) List() []*models.SpecInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	specs := make([]*models.SpecInfo, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec)
	}

	return specs
}

// GetExpired returns all expired specifications
func (r *Registry) GetExpired() []*models.SpecInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var expired []*models.SpecInfo
	for _, spec := range r.specs {
		if r.isExpired(spec) {
			expired = append(expired, spec)
		}
	}

	return expired
}

// Events returns the event channel for spec changes
func (r *Registry) Events() <-chan SpecEvent {
	return r.events
}

// StartCleanup starts a background goroutine to clean up expired specs
func (r *Registry) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.cleanupExpired()
			}
		}
	}()
}

// Stats returns statistics about the registry
func (r *Registry) Stats() map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	stats := map[string]interface{}{
		"totalSpecs":   len(r.specs),
		"expiredSpecs": len(r.GetExpired()),
		"services":     make([]string, 0, len(r.specs)),
	}

	services := make([]string, 0, len(r.specs))
	for serviceName := range r.specs {
		services = append(services, serviceName)
	}
	stats["services"] = services

	return stats
}

// isExpired checks if a specification has exceeded its TTL
func (r *Registry) isExpired(spec *models.SpecInfo) bool {
	if spec.TTL <= 0 {
		return false // No expiration
	}
	return time.Since(spec.FetchedAt) > spec.TTL
}

// emitEvent sends an event to the event channel (non-blocking)
func (r *Registry) emitEvent(event SpecEvent) {
	select {
	case r.events <- event:
	default:
		r.logger.Warn("Event channel full, dropping event",
			zap.String("eventType", string(event.Type)),
			zap.String("serviceName", event.ServiceName))
	}
}

// cleanupExpired removes expired specifications that have been expired for too long
func (r *Registry) cleanupExpired() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()
	for serviceName, spec := range r.specs {
		if r.isExpired(spec) {
			// Only remove specs that have been expired for more than their TTL duration
			expiredFor := now.Sub(spec.FetchedAt.Add(spec.TTL))
			if expiredFor > spec.TTL {
				delete(r.specs, serviceName)
				r.logger.Info("Cleaned up expired spec",
					zap.String("serviceName", serviceName),
					zap.Duration("expiredFor", expiredFor))

				r.emitEvent(SpecEvent{
					Type:        SpecEventRemoved,
					ServiceName: serviceName,
					Timestamp:   now,
				})
			}
		}
	}
}