// Package api implements the toy shop's /api/v1 HTTP endpoints over
// oapi-codegen's strict std-http server, mounted through apiroutes.Mount.
//
// oapi-codegen is invoked via go:generate rather than genx because it generates
// server stubs and models directly rather than a genx domain file.
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml

import (
	"context"
	"slices"

	"github.com/epistemer/auth"
	"github.com/epistemer/handlers/examples/shop/internal/ids"
	"github.com/epistemer/handlers/examples/shop/internal/models"
	"github.com/epistemer/handlers/examples/shop/internal/shop"
)

// Server implements StrictServerInterface for the shop API.
type Server struct {
	Auth  *auth.Auth
	Store *shop.Store
}

// SignIn starts a bearer-token sign-in with email and password, sending an
// email code challenge when a second factor is required.
func (s *Server) SignIn(ctx context.Context, req SignInRequestObject) (SignInResponseObject, error) {
	f, err := s.Auth.SignIn(ctx, string(req.Body.Email), auth.Bearer())
	if err == nil {
		f, err = s.Auth.Prove(ctx, f.Token, auth.Password(req.Body.Password))
	}
	if err != nil {
		return nil, err
	}
	if !f.Verified && slices.Contains(f.Offers, auth.MethodEmailCode) {
		if _, err := s.Auth.Challenge(ctx, f.Token, auth.MethodEmailCode); err != nil {
			return nil, err
		}
		next := "code"
		return SignIn200JSONResponse{
			Next: &next,
			Flow: &f.Token,
		}, nil
	}
	res, err := s.Auth.Finish(ctx, f.Token)
	if err != nil {
		return nil, err
	}
	return SignIn200JSONResponse{
		Access:   &res.Access,
		Refresh:  &res.Refresh,
		Verified: &res.Session.Verified,
	}, nil
}

// SignInCode verifies the emailed code for a pending API sign-in flow and
// issues bearer tokens.
func (s *Server) SignInCode(ctx context.Context, req SignInCodeRequestObject) (SignInCodeResponseObject, error) {
	f, err := s.Auth.Prove(ctx, req.Body.Flow, auth.EmailCode(req.Body.Code))
	if err != nil {
		return nil, err
	}
	res, err := s.Auth.Finish(ctx, f.Token)
	if err != nil {
		return nil, err
	}
	return SignInCode200JSONResponse{
		Access:   &res.Access,
		Refresh:  &res.Refresh,
		Verified: &res.Session.Verified,
	}, nil
}

// GetMe returns the authenticated session's user details (x-auth: session).
func (s *Server) GetMe(ctx context.Context, _ GetMeRequestObject) (GetMeResponseObject, error) {
	sess, ok := auth.From(ctx)
	if !ok {
		return nil, auth.ErrNoSession
	}
	return GetMe200JSONResponse{
		Id:       string(sess.User),
		Verified: sess.Verified,
	}, nil
}

// ListOrders returns orders, optionally filtered by status.
func (s *Server) ListOrders(_ context.Context, req ListOrdersRequestObject) (ListOrdersResponseObject, error) {
	var statusFilter *models.OrderStatus
	if req.Params.Status != nil {
		st := models.OrderStatus(*req.Params.Status)
		statusFilter = &st
	}
	orders := s.Store.Orders(statusFilter)
	out := make(ListOrders200JSONResponse, len(orders))
	for i, o := range orders {
		out[i] = toAPIOrder(o)
	}
	return out, nil
}

// GetOrder returns a single order by ID, or shop.ErrOrderNotFound.
func (s *Server) GetOrder(_ context.Context, req GetOrderRequestObject) (GetOrderResponseObject, error) {
	o, err := s.Store.Order(ids.OrderID(req.OrderId))
	if err != nil {
		return nil, err
	}
	return GetOrder200JSONResponse(toAPIOrder(o)), nil
}

func toAPIOrder(o shop.Order) Order {
	return Order{
		Id:     int64(o.ID),
		Status: Status(o.Status),
		Total:  o.Total,
	}
}
