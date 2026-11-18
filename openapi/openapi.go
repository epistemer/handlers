// Package openapi adapts oapi-codegen's strict std-http server to
// handlers' route table, guard, error writer, and kin-openapi request
// validation.
package openapi

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/epistemer/handlers"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

// Router implements oapi-codegen's ServeMux interface.
type Router struct {
	mux        *http.ServeMux
	mount      *handlers.Mount
	routes     []handlers.Route
	routeIndex map[string]int
	spec       *openapi3.T
	writer     handlers.ErrorWriter
}

// Mount calls register with a Router over mux, then panics if a route of
// routes wasn't registered. It panics on a nil mux, spec or register.
func Mount(mux *http.ServeMux, routes []handlers.Route, g handlers.Guard, spec *openapi3.T, register func(*Router), opts ...handlers.Option) {
	if mux == nil {
		panic("handlers/openapi: nil mux")
	}
	if spec == nil {
		panic("handlers/openapi: nil spec")
	}
	if register == nil {
		panic("handlers/openapi: nil register")
	}

	m := handlers.NewMount(mux, routes, g, opts...)
	writer := m.Writer()
	if writer == nil {
		writer = handlers.WriteJSON
	}

	idx := make(map[string]int, len(routes))
	for i, rt := range routes {
		idx[rt.Method+" "+rt.Prefix+rt.Pattern] = i
	}

	rt := &Router{
		mux:        mux,
		mount:      m,
		routes:     routes,
		routeIndex: idx,
		spec:       spec,
		writer:     writer,
	}
	register(rt)
	m.Done()
}

// HandleFunc registers pattern from oapi-codegen (<METHOD> <prefix><path>)
// on the underlying Mount with request validation against the OpenAPI spec.
func (r *Router) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	idx, ok := r.routeIndex[pattern]
	if !ok {
		panic(fmt.Sprintf("handlers/openapi: %s: no route", pattern))
	}
	route := r.routes[idx]

	if r.spec.Paths == nil {
		panic(fmt.Sprintf("handlers/openapi: %s: path %q missing from spec", pattern, route.Pattern))
	}
	pathItem := r.spec.Paths.Value(route.Pattern)
	if pathItem == nil {
		panic(fmt.Sprintf("handlers/openapi: %s: path %q missing from spec", pattern, route.Pattern))
	}
	op := pathItem.GetOperation(route.Method)
	if op == nil {
		panic(fmt.Sprintf("handlers/openapi: %s: operation %s missing from path item", pattern, route.Method))
	}

	var hh http.Handler
	if h != nil {
		hh = http.HandlerFunc(h)
	}

	paramNames := collectPathParamNames(pathItem, op, route.Params)
	hasMultipart, hasNonMultipart := classifyRequestBody(op)
	kinRoute := &routers.Route{
		Spec:      r.spec,
		Path:      route.Pattern,
		PathItem:  pathItem,
		Method:    route.Method,
		Operation: op,
	}
	disableDetails := !r.spec.IsOpenAPI31OrLater()

	validate := func(req *http.Request) error {
		var pathParams map[string]string
		if len(paramNames) > 0 {
			pathParams = make(map[string]string, len(paramNames))
			for _, name := range paramNames {
				pathParams[name] = req.PathValue(name)
			}
		}

		excludeBody := false
		if hasMultipart {
			if !hasNonMultipart || isMultipartRequest(req) {
				excludeBody = true
			}
		}

		valOpts := &openapi3filter.Options{
			ExcludeRequestBody: excludeBody,
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		}
		if disableDetails {
			valOpts.SchemaValidationOptions = []openapi3.SchemaValidationOption{openapi3.DisableSchemaErrorDetails()}
		}

		err := openapi3filter.ValidateRequest(req.Context(), &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      kinRoute,
			Options:    valOpts,
		})
		if err == nil {
			return nil
		}
		return mapValidationError(err)
	}

	r.mount.HandleHTTP(idx, hh, validate)
}

// ServeHTTP delegates to the mux passed to Mount.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// RequestError handles oapi-codegen parameter/body binding errors, writing
// ErrTooLarge for *http.MaxBytesError or ErrInvalidRequest otherwise, and
// logging the cause at debug level.
func (r *Router) RequestError(w http.ResponseWriter, req *http.Request, err error) {
	slog.Default().DebugContext(req.Context(), "handlers/openapi: request error", "err", err)
	var maxErr *http.MaxBytesError
	if stderrors.As(err, &maxErr) {
		r.writer(w, req, handlers.ErrTooLarge.Wrap(err))
		return
	}
	r.writer(w, req, handlers.ErrInvalidRequest.Wrap(err))
}

// WriteError writes a handler error through Mount's configured ErrorWriter.
func (r *Router) WriteError(w http.ResponseWriter, req *http.Request, err error) {
	r.writer(w, req, err)
}

func collectPathParamNames(pi *openapi3.PathItem, op *openapi3.Operation, routeParams []handlers.Param) []string {
	seen := make(map[string]bool)
	var names []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, ref := range pi.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == openapi3.ParameterInPath {
			add(ref.Value.Name)
		}
	}
	for _, ref := range op.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == openapi3.ParameterInPath {
			add(ref.Value.Name)
		}
	}
	for _, p := range routeParams {
		add(p.Name)
	}
	return names
}

func classifyRequestBody(op *openapi3.Operation) (hasMultipart, hasNonMultipart bool) {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return false, false
	}
	for mime := range op.RequestBody.Value.Content {
		base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mime)), ";")
		if strings.TrimSpace(base) == "multipart/form-data" {
			hasMultipart = true
		} else {
			hasNonMultipart = true
		}
	}
	return hasMultipart, hasNonMultipart
}

func isMultipartRequest(req *http.Request) bool {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Type"))), ";")
	return strings.TrimSpace(base) == "multipart/form-data"
}

func mapValidationError(err error) error {
	var maxErr *http.MaxBytesError
	if stderrors.As(err, &maxErr) {
		return handlers.ErrTooLarge.Wrap(err)
	}

	var multi openapi3.MultiError
	if stderrors.As(err, &multi) && len(multi) > 0 {
		err = multi[0]
	}

	cause := sanitizeError(err)

	var reqErr *openapi3filter.RequestError
	if stderrors.As(err, &reqErr) {
		if reqErr.Parameter != nil && reqErr.Parameter.Name != "" {
			return handlers.ErrInvalidField.Wrap(cause, reqErr.Parameter.Name)
		}
	}

	var schemaErr *openapi3.SchemaError
	if stderrors.As(err, &schemaErr) {
		if ptr := schemaErr.JSONPointer(); len(ptr) > 0 {
			return handlers.ErrInvalidField.Wrap(cause, formatJSONPointer(ptr))
		}
	}

	return handlers.ErrInvalidRequest.Wrap(cause)
}

func formatJSONPointer(segments []string) string {
	var b strings.Builder
	for _, seg := range segments {
		b.WriteByte('/')
		seg = strings.ReplaceAll(seg, "~", "~0")
		seg = strings.ReplaceAll(seg, "/", "~1")
		b.WriteString(seg)
	}
	return b.String()
}

func sanitizeError(err error) error {
	stripParseValues(err)
	return err
}

func stripParseValues(err error) {
	for err != nil {
		if pe, ok := err.(*openapi3filter.ParseError); ok {
			pe.Value = nil
			err = pe.Cause
			continue
		}
		if re, ok := err.(*openapi3filter.RequestError); ok {
			err = re.Err
			continue
		}
		err = stderrors.Unwrap(err)
	}
}
