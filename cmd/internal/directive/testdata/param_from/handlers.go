package shop

import "net/http"

type H struct{}

//h:key items.list
//h:method GET
//h:path /items
//h:desc Items
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key items.create
//h:method POST
//h:path /items
//h:desc Create an item
func (h *H) Create(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key items.show
//h:method GET
//h:path /items/{id}
//h:desc Show an item
//h:param id from items.show
func (h *H) Show(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

//h:key items.edit
//h:method GET
//h:path /items/{id}/edit/{part}/{rev}/{tag}
//h:desc Edit an item
//h:param id from items.missing
//h:param part from items.create label Part
//h:param rev from items.show
//h:param tag from Bad.Key
func (h *H) Edit(w http.ResponseWriter, r *http.Request, id int64, part, rev, tag string) error { return nil }

//h:key items.copy
//h:method GET
//h:path /items/{id}/copy
//h:desc Copy an item
//h:param id from items.list label Item
func (h *H) Copy(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

//h:key items.save
//h:method POST
//h:path /items/{id}
//h:desc Save an item
//h:search
//h:view
func (h *H) Save(w http.ResponseWriter, r *http.Request, id int64) error { return nil }
