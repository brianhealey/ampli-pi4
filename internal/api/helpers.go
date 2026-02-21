// Package api implements the HTTP REST API for AmpliPi.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// Handlers holds dependencies for all HTTP handlers.
type Handlers struct {
	ctrl   Controller
	events EventBus
}

// Controller is the interface the handlers use to interact with the system state.
type Controller interface {
	State() models.State
	GetSources() []models.Source
	GetSource(id int) (*models.Source, *models.AppError)
	SetSource(ctx context.Context, id int, upd models.SourceUpdate) (models.State, *models.AppError)
	GetZones() []models.Zone
	GetZone(id int) (*models.Zone, *models.AppError)
	SetZone(ctx context.Context, id int, upd models.ZoneUpdate) (models.State, *models.AppError)
	SetZones(ctx context.Context, req models.MultiZoneUpdate) (models.State, *models.AppError)
	GetGroups() []models.Group
	GetGroup(id int) (*models.Group, *models.AppError)
	CreateGroup(ctx context.Context, req models.GroupUpdate) (models.State, *models.AppError)
	SetGroup(ctx context.Context, id int, upd models.GroupUpdate) (models.State, *models.AppError)
	DeleteGroup(ctx context.Context, id int) (models.State, *models.AppError)
	GetStreams() []models.Stream
	GetStream(id int) (*models.Stream, *models.AppError)
	CreateStream(ctx context.Context, req models.StreamCreate) (models.State, *models.AppError)
	SetStream(ctx context.Context, id int, upd models.StreamUpdate) (models.State, *models.AppError)
	DeleteStream(ctx context.Context, id int) (models.State, *models.AppError)
	ExecStreamCommand(ctx context.Context, id int, cmd string) (models.State, *models.AppError)
	GetPresets() []models.Preset
	GetPreset(id int) (*models.Preset, *models.AppError)
	CreatePreset(ctx context.Context, req models.PresetCreate) (models.State, *models.AppError)
	SetPreset(ctx context.Context, id int, upd models.PresetUpdate) (models.State, *models.AppError)
	DeletePreset(ctx context.Context, id int) (models.State, *models.AppError)
	LoadPreset(ctx context.Context, id int) (models.State, *models.AppError)
	GetInfo() models.Info
	FactoryReset(ctx context.Context) (models.State, *models.AppError)
	LoadConfig(ctx context.Context, incoming models.State) (models.State, *models.AppError)
	TestPreamp(ctx context.Context) (map[string]interface{}, error)
	TestFans(ctx context.Context) (map[string]interface{}, error)
}

// EventBus is the interface for subscribing to state change events.
type EventBus interface {
	Subscribe(id string) <-chan models.State
	Unsubscribe(id string)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes an AppError as a JSON response.
func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	if appErr, ok := err.(*models.AppError); ok {
		w.WriteHeader(appErr.Status)
		_ = json.NewEncoder(w).Encode(appErr)
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(models.ErrInternal(err.Error()))
}

// intParam reads an integer path parameter by name.
func intParam(r *http.Request, name string) (int, error) {
	s := chi.URLParam(r, name)
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, models.ErrBadRequest("invalid " + name + " parameter")
	}
	return n, nil
}
