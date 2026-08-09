package tests

import (
	"errors"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
)

type MockedMediaMarkerRepo struct {
	model.MediaMarkerRepository
	Data map[string]*model.MediaMarker
	Err  bool
}

func CreateMockedMediaMarkerRepo() *MockedMediaMarkerRepo {
	return &MockedMediaMarkerRepo{Data: map[string]*model.MediaMarker{}}
}

func (m *MockedMediaMarkerRepo) SetError(err bool) {
	m.Err = err
}

func (m *MockedMediaMarkerRepo) Get(id string) (*model.MediaMarker, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	if d, ok := m.Data[id]; ok {
		return d, nil
	}
	return nil, model.ErrNotFound
}

func (m *MockedMediaMarkerRepo) GetAll(...model.QueryOptions) (model.MediaMarkers, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	var all model.MediaMarkers
	for _, d := range m.Data {
		all = append(all, *d)
	}
	return all, nil
}

func (m *MockedMediaMarkerRepo) GetByItemID(itemID string) (model.MediaMarkers, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	var res model.MediaMarkers
	for _, d := range m.Data {
		if d.ItemID == itemID {
			res = append(res, *d)
		}
	}
	return res, nil
}

func (m *MockedMediaMarkerRepo) Put(marker *model.MediaMarker) error {
	if m.Err {
		return errors.New("error")
	}
	if marker.ID == "" {
		marker.ID = id.NewRandom()
	}
	if marker.ItemType == "" {
		marker.ItemType = model.MediaMarkerItemTypeMediaFile
	}
	m.Data[marker.ID] = marker
	return nil
}

func (m *MockedMediaMarkerRepo) Delete(id string) error {
	if m.Err {
		return errors.New("error")
	}
	if _, found := m.Data[id]; !found {
		return model.ErrNotFound
	}
	delete(m.Data, id)
	return nil
}
