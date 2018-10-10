package yext

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
)

const (
	entityPath         = "entities"
	EntityListMaxLimit = 50
)

type EntityService struct {
	client   *Client
	registry Registry
}

type EntityListOptions struct {
	ListOptions
	SearchID            string
	ResolvePlaceholders bool
}

type EntityListResponse struct {
	Count        int           `json:"count"`
	Entities     []interface{} `json:"entities"`
	typedEntites []Entity
	PageToken    string `json:"pageToken"`
}

func (e *EntityService) RegisterDefaultEntities() {
	e.registry = make(Registry)
	e.RegisterEntity(ENTITYTYPE_LOCATION, &LocationEntity{})
	e.RegisterEntity(ENTITYTYPE_EVENT, &Event{})
}

func (e *EntityService) RegisterEntity(t EntityType, entity interface{}) {
	e.registry.Register(string(t), entity)
}

func (e *EntityService) CreateEntity(t EntityType) (interface{}, error) {
	return e.registry.Create(string(t))
}

func (e *EntityService) toEntityTypes(entities []interface{}) ([]Entity, error) {
	var types = []Entity{}
	for _, entityInterface := range entities {
		entity, err := e.toEntityType(entityInterface)
		if err != nil {
			return nil, err
		}
		types = append(types, entity)
	}
	return types, nil
}

func (e *EntityService) toEntityType(entity interface{}) (Entity, error) {
	// Determine Entity Type
	var entityValsByKey = entity.(map[string]interface{})
	meta, ok := entityValsByKey["meta"]
	if !ok {
		return nil, fmt.Errorf("Unable to find meta attribute in %v", entityValsByKey)
	}

	var metaByKey = meta.(map[string]interface{})
	entityType, ok := metaByKey["entityType"]
	if !ok {
		return nil, fmt.Errorf("Unable to find entityType attribute in %v", metaByKey)
	}

	// TODO: Re-examine what happens when we get an error here. Do we want to procede with a generic type?
	entityObj, err := e.CreateEntity(EntityType(entityType.(string)))
	if err != nil {
		return nil, err
	}

	// Convert into struct of Entity Type
	entityJSON, err := json.Marshal(entityValsByKey)
	if err != nil {
		return nil, fmt.Errorf("Marshaling entity to JSON: %s", err)
	}

	err = json.Unmarshal(entityJSON, &entityObj)
	if err != nil {
		return nil, fmt.Errorf("Unmarshaling entity JSON: %s", err)
	}
	return entityObj.(Entity), nil
}

// TODO: Paging is not working here. Waiting on techops
// TODO: Add List for SearchID (similar to location-service). Follow up with Techops to see if SearchID is implemented
func (e *EntityService) ListAll(opts *EntityListOptions) ([]Entity, error) {
	var entities []Entity
	if opts == nil {
		opts = &EntityListOptions{}
	}
	opts.ListOptions = ListOptions{Limit: EntityListMaxLimit}
	var lg tokenListRetriever = func(listOptions *ListOptions) (string, error) {
		opts.ListOptions = *listOptions
		resp, _, err := e.List(opts)
		if err != nil {
			return "", err
		}

		entities = append(entities, resp.typedEntites...)
		return resp.PageToken, err
	}

	if err := tokenListHelper(lg, &opts.ListOptions); err != nil {
		return nil, err
	} else {
		return entities, nil
	}
}

func (e *EntityService) List(opts *EntityListOptions) (*EntityListResponse, *Response, error) {
	var (
		requrl = entityPath
		err    error
	)

	if opts != nil {
		requrl, err = addEntityListOptions(requrl, opts)
		if err != nil {
			return nil, nil, err
		}
	}

	if opts != nil {
		requrl, err = addListOptions(requrl, &opts.ListOptions)
		if err != nil {
			return nil, nil, err
		}
	}

	v := &EntityListResponse{}
	r, err := e.client.DoRequest("GET", requrl, v)
	if err != nil {
		return nil, r, err
	}

	typedEntities, err := e.toEntityTypes(v.Entities)
	if err != nil {
		return nil, r, err
	}
	entities := []Entity{}
	for _, entity := range typedEntities {
		setNilIsEmpty(entity)
		entities = append(entities, entity)
	}
	v.typedEntites = entities

	return v, r, nil
}

func addEntityListOptions(requrl string, opts *EntityListOptions) (string, error) {
	if opts == nil {
		return requrl, nil
	}

	u, err := url.Parse(requrl)
	if err != nil {
		return "", err
	}

	q := u.Query()
	if opts.SearchID != "" {
		q.Add("searchId", opts.SearchID)
	}
	if opts.ResolvePlaceholders {
		q.Add("resolvePlaceholders", "true")
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (e *EntityService) Get(id string) (Entity, *Response, error) {
	var v map[string]interface{}
	r, err := e.client.DoRequest("GET", fmt.Sprintf("%s/%s", entityPath, id), &v)
	if err != nil {
		return nil, r, err
	}

	entity, err := e.toEntityType(v)
	if err != nil {
		return nil, r, err
	}

	setNilIsEmpty(entity)

	return entity, r, nil
}

func setNilIsEmpty(i interface{}) {
	m := reflect.ValueOf(i).MethodByName("SetNilIsEmpty")
	if m.IsValid() {
		m.Call([]reflect.Value{reflect.ValueOf(true)})
	}
}

func getNilIsEmpty(i interface{}) bool {
	m := reflect.ValueOf(i).MethodByName("GetNilIsEmpty")
	if m.IsValid() {
		values := m.Call([]reflect.Value{})
		if len(values) == 1 {
			return values[0].Interface().(bool)
		}
	}
	return false
}

// TODO: Currently an error with API. Need to test this
func (e *EntityService) Create(y Entity) (*Response, error) {
	var requrl = entityPath
	u, err := url.Parse(requrl)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Add("entityType", string(y.GetEntityType()))
	u.RawQuery = q.Encode()
	r, err := e.client.DoRequestJSON("POST", u.String(), y, nil)
	if err != nil {
		return r, err
	}

	return r, nil
}

// TODO: There is an outstanding techops QA issue to allow the Id in the request but we may have to remove other things like account
func (e *EntityService) Edit(y Entity) (*Response, error) {
	r, err := e.client.DoRequestJSON("PUT", fmt.Sprintf("%s/%s", entityPath, y.GetEntityId()), y, nil)
	if err != nil {
		return r, err
	}

	return r, nil
}
