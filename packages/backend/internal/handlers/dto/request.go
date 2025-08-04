package dto

import (
	"time"

	"github.com/konflux-ci/kite/internal/models"
)

// DTOs (Data Transfer Objects)
// These allow us to carry and format data between layers or services, without embedding any business logic.

// For requests

type ScopeReqBody struct {
	ResourceType      string `json:"resourceType" binding:"required"`
	ResourceName      string `json:"resourceName" binding:"required"`
	ResourceNamespace string `json:"resourceNamespace"`
}

type CreateIssueRequest struct {
	Title       string              `json:"title" binding:"required"`
	Description string              `json:"description" binding:"required"`
	Severity    models.Severity     `json:"severity" binding:"required"`
	IssueType   models.IssueType    `json:"issueType" binding:"required"`
	State       models.IssueState   `json:"state"`
	Namespace   string              `json:"namespace" binding:"required"`
	Scope       ScopeReqBody        `json:"scope" binding:"required"`
	Links       []CreateLinkRequest `json:"links"`
}

type CreateLinkRequest struct {
	Title string `json:"title" binding:"required"`
	URL   string `json:"url" binding:"required"`
}

type UpdateIssueRequest struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Severity    models.Severity     `json:"severity"`
	IssueType   models.IssueType    `json:"issueType"`
	State       models.IssueState   `json:"state"`
	Namespace   string              `json:"namespace"`
	Scope       ScopeReqBody        `json:"scope"`
	Links       []CreateLinkRequest `json:"links"`
	ResolvedAt  time.Time           `json:"resolvedAt"`
}

// This interface allows for both create and update request structs
// to be used in the same method.
type IssuePayload interface {
	GetTitle() string
	GetDescription() string
	GetSeverity() models.Severity
	GetIssueType() models.IssueType
	GetState() models.IssueState
	GetLinks() []CreateLinkRequest
	GetResolvedAt() time.Time
	GetNamespace() string
	GetScope() ScopeReqBody
}

func (c CreateIssueRequest) GetTitle() string               { return c.Title }
func (c CreateIssueRequest) GetDescription() string         { return c.Description }
func (c CreateIssueRequest) GetSeverity() models.Severity   { return c.Severity }
func (c CreateIssueRequest) GetIssueType() models.IssueType { return c.IssueType }
func (c CreateIssueRequest) GetState() models.IssueState    { return c.State }
func (c CreateIssueRequest) GetLinks() []CreateLinkRequest  { return c.Links }
func (c CreateIssueRequest) GetScope() ScopeReqBody         { return c.Scope }
func (c CreateIssueRequest) GetNamespace() string           { return c.Namespace }

// Create requests don't have the resolved_at value.
// For this interface we'll return an empty time value.
func (c CreateIssueRequest) GetResolvedAt() time.Time { return time.Time{} }

func (u UpdateIssueRequest) GetTitle() string               { return u.Title }
func (u UpdateIssueRequest) GetDescription() string         { return u.Description }
func (u UpdateIssueRequest) GetSeverity() models.Severity   { return u.Severity }
func (u UpdateIssueRequest) GetIssueType() models.IssueType { return u.IssueType }
func (u UpdateIssueRequest) GetState() models.IssueState    { return u.State }
func (u UpdateIssueRequest) GetLinks() []CreateLinkRequest  { return u.Links }
func (u UpdateIssueRequest) GetScope() ScopeReqBody         { return u.Scope }
func (u UpdateIssueRequest) GetNamespace() string           { return u.Namespace }
func (u UpdateIssueRequest) GetResolvedAt() time.Time       { return u.ResolvedAt }
