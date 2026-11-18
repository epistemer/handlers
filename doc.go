// Package handlers is neozp's route package: it turns admin routes declared
// by comment directives on handler methods, and API routes declared in an
// OpenAPI file, into one route table, with registration on net/http, labels
// for i18n and permissions for auth.
//
// The design is docs/brief.md; the contract will be docs/interface.md.
package handlers
