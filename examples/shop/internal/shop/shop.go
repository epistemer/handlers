// Package shop implements the toy shop's domain model, in-memory catalog and
// order store, declared domain errors, and auth user directory and mailer.
package shop

import (
	"context"
	"slices"
	"sync"

	"github.com/epistemer/auth"
	"github.com/epistemer/errors"
	"github.com/epistemer/handlers/examples/shop/internal/ids"
	"github.com/epistemer/handlers/examples/shop/internal/models"
	"github.com/shopspring/decimal"
)

// Domain errors declared with errors.Key so HTTP surfaces render localized
// messages to users without exposing internal error details.
var (
	ErrProductNotFound       = errors.Key("errors.shop.product_not_found", "product not found", errors.NotFound)
	ErrOrderNotFound         = errors.Key("errors.shop.order_not_found", "order not found", errors.NotFound)
	ErrOrderAlreadyCancelled = errors.Key("errors.shop.order_already_cancelled", "order already cancelled", errors.Conflict)
)

// Product is a toy sold by the shop.
type Product struct {
	ID    ids.ProductID
	SKU   string
	Name  string
	Price decimal.Decimal
}

// Order is a customer order with a decimal total and a status enum.
type Order struct {
	ID     ids.OrderID
	Status models.OrderStatus
	Total  decimal.Decimal
}

// Store holds products and orders in memory for the example app.
type Store struct {
	mu       sync.RWMutex
	products []Product
	orders   []Order
}

// NewStore returns a Store seeded with sample products and orders.
func NewStore() *Store {
	return &Store{
		products: []Product{
			{ID: 1, SKU: "TOY-TRAIN", Name: "Wooden Train", Price: decimal.RequireFromString("24.50")},
			{ID: 2, SKU: "TOY-BLOCKS", Name: "Building Blocks", Price: decimal.RequireFromString("19.99")},
		},
		orders: []Order{
			{ID: 1, Status: models.OrderStatusPaid, Total: decimal.RequireFromString("24.50")},
			{ID: 2, Status: models.OrderStatusPending, Total: decimal.RequireFromString("39.98")},
		},
	}
}

// Products returns a snapshot of all products ordered by ID.
func (s *Store) Products() []Product {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.products)
}

// Product returns the product with id or ErrProductNotFound.
func (s *Store) Product(id ids.ProductID) (Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.products {
		if p.ID == id {
			return p, nil
		}
	}
	return Product{}, ErrProductNotFound
}

// Orders returns a snapshot of orders, optionally filtered by status.
func (s *Store) Orders(status *models.OrderStatus) []Order {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Order, 0, len(s.orders))
	for _, o := range s.orders {
		if status != nil && o.Status != *status {
			continue
		}
		out = append(out, o)
	}
	return out
}

// Order returns the order with id or ErrOrderNotFound.
func (s *Store) Order(id ids.OrderID) (Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, o := range s.orders {
		if o.ID == id {
			return o, nil
		}
	}
	return Order{}, ErrOrderNotFound
}

// CancelOrder marks the order with id as cancelled, returning ErrOrderNotFound
// if absent or ErrOrderAlreadyCancelled if it was already cancelled.
func (s *Store) CancelOrder(id ids.OrderID) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.orders {
		if s.orders[i].ID != id {
			continue
		}
		if s.orders[i].Status == models.OrderStatusCancelled {
			return Order{}, ErrOrderAlreadyCancelled
		}
		s.orders[i].Status = models.OrderStatusCancelled
		return s.orders[i], nil
	}
	return Order{}, ErrOrderNotFound
}

// Directory implements auth.Directory in memory for the example app.
type Directory struct {
	mu     sync.Mutex
	users  map[string]auth.User // keyed by login
	emails map[auth.UserID]string
}

// NewDirectory returns an empty user directory.
func NewDirectory() *Directory {
	return &Directory{
		users:  make(map[string]auth.User),
		emails: make(map[auth.UserID]string),
	}
}

// Add registers a user under login with an optional email address.
func (d *Directory) Add(id auth.UserID, login, name, email string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.users[login] = auth.User{ID: id, Name: name}
	d.emails[id] = email
}

// Find looks up a user by login.
func (d *Directory) Find(_ context.Context, login string) (auth.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if u, ok := d.users[login]; ok {
		return u, nil
	}
	return auth.User{}, auth.ErrNoUser
}

// Get looks up a user by ID.
func (d *Directory) Get(_ context.Context, id auth.UserID) (auth.User, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, u := range d.users {
		if u.ID == id {
			return u, nil
		}
	}
	return auth.User{}, auth.ErrNoUser
}

// Address returns the user's address for ch.
func (d *Directory) Address(_ context.Context, id auth.UserID, ch auth.Channel) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ch == auth.ChannelEmail {
		return d.emails[id], nil
	}
	return "", nil
}

// Outbox records the last message sent to each address.
type Outbox struct {
	mu   sync.Mutex
	last map[string]auth.Message
}

// NewOutbox returns an empty outbox.
func NewOutbox() *Outbox {
	return &Outbox{last: make(map[string]auth.Message)}
}

// Send records m by recipient address.
func (o *Outbox) Send(_ context.Context, m auth.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.last[m.To] = m
	return nil
}

// Last returns the last message sent to to.
func (o *Outbox) Last(to string) (auth.Message, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	m, ok := o.last[to]
	return m, ok
}
