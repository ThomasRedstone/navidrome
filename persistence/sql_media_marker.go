package persistence

import (
	"context"
	"errors"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/deluan/rest"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/pocketbase/dbx"
)

type mediaMarkerRepository struct {
	sqlRepository
}

func NewMediaMarkerRepository(ctx context.Context, db dbx.Builder) model.MediaMarkerRepository {
	r := &mediaMarkerRepository{}
	r.ctx = ctx
	r.db = db
	r.registerModel(&model.MediaMarker{}, map[string]filterFunc{
		"item_id":   eqFilter,
		"item_type": eqFilter,
		"kind":      eqFilter,
		"source":    eqFilter,
	})
	return r
}

func (r *mediaMarkerRepository) Get(id string) (*model.MediaMarker, error) {
	sel := r.newSelect().Where(Eq{"id": id}).Columns("*")
	res := model.MediaMarker{}
	err := r.queryOne(sel, &res)
	return &res, err
}

func (r *mediaMarkerRepository) GetAll(options ...model.QueryOptions) (model.MediaMarkers, error) {
	sel := r.newSelect(options...).Columns("*")
	res := model.MediaMarkers{}
	err := r.queryAll(sel, &res)
	return res, err
}

// GetByItemID returns every marker for a single item (e.g. all markers on one media file),
// ordered by start_ms so callers can render them in playback order.
func (r *mediaMarkerRepository) GetByItemID(itemID string) (model.MediaMarkers, error) {
	sel := r.newSelect().
		Where(Eq{"item_id": itemID, "item_type": model.MediaMarkerItemTypeMediaFile}).
		OrderBy("start_ms").
		Columns("*")
	res := model.MediaMarkers{}
	err := r.queryAll(sel, &res)
	return res, err
}

func (r *mediaMarkerRepository) Put(m *model.MediaMarker) error {
	m.UpdatedAt = time.Now()
	if m.ID == "" {
		m.CreatedAt = time.Now()
		m.ID = id.NewRandom()
	}
	if m.ItemType == "" {
		m.ItemType = model.MediaMarkerItemTypeMediaFile
	}
	_, err := r.put(m.ID, m)
	return err
}

func (r *mediaMarkerRepository) Delete(id string) error {
	return r.delete(Eq{"id": id})
}

func (r *mediaMarkerRepository) CountAll(options ...model.QueryOptions) (int64, error) {
	sel := r.newSelect()
	return r.count(sel, options...)
}

func (r *mediaMarkerRepository) Count(options ...rest.QueryOptions) (int64, error) {
	return r.CountAll(r.parseRestOptions(r.ctx, options...))
}

func (r *mediaMarkerRepository) EntityName() string {
	return "media_marker"
}

func (r *mediaMarkerRepository) NewInstance() any {
	return &model.MediaMarker{}
}

func (r *mediaMarkerRepository) Read(id string) (any, error) {
	return r.Get(id)
}

func (r *mediaMarkerRepository) ReadAll(options ...rest.QueryOptions) (any, error) {
	return r.GetAll(r.parseRestOptions(r.ctx, options...))
}

func (r *mediaMarkerRepository) Save(entity any) (string, error) {
	t := entity.(*model.MediaMarker)
	err := r.Put(t)
	if errors.Is(err, model.ErrNotFound) {
		return "", rest.ErrNotFound
	}
	return t.ID, err
}

func (r *mediaMarkerRepository) Update(id string, entity any, cols ...string) error {
	t := entity.(*model.MediaMarker)
	t.ID = id
	err := r.Put(t)
	if errors.Is(err, model.ErrNotFound) {
		return rest.ErrNotFound
	}
	return err
}

var _ model.MediaMarkerRepository = (*mediaMarkerRepository)(nil)
var _ rest.Repository = (*mediaMarkerRepository)(nil)
var _ rest.Persistable = (*mediaMarkerRepository)(nil)
