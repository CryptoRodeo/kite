package http

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konflux-ci/kite/internal/config"
	"github.com/konflux-ci/kite/internal/handlers/dto"
	"github.com/konflux-ci/kite/internal/models"
	"github.com/konflux-ci/kite/internal/services"
	"github.com/sirupsen/logrus"
)

type WebhookHandler struct {
	issueService services.IssueServiceInterface // IssueService instance
	logger       *logrus.Logger                 // Logging Instance
}

// NewWebhookHandler returns a new handler for the webhooks route
func NewWebhookHandler(issueService services.IssueServiceInterface, logger *logrus.Logger) *WebhookHandler {
	return &WebhookHandler{
		issueService: issueService,
		logger:       logger,
	}
}

type PipelineFailureRequest struct {
	PipelineName  string `json:"pipelineName" binding:"required"`
	Namespace     string `json:"namespace" binding:"required"`
	Severity      string `json:"severity"`
	FailureReason string `json:"failureReason" binding:"required"`
	RunID         string `json:"runId"`
	LogsURL       string `json:"logsUrl"`
}

type PipelineSuccessRequest struct {
	PipelineName string `json:"pipelineName" binding:"required"`
	Namespace    string `json:"namespace" binding:"required"`
}

// PipelineFailure handles pipeline failure webhooks
func (h *WebhookHandler) PipelineFailure(c *gin.Context) {
	var req PipelineFailureRequest
	// Check if the request binds to proper JSON, in the format specified
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required fields", "details": err.Error()})
		return
	}

	// Format issue data
	logsURL := req.LogsURL
	if logsURL == "" {
		baseURL := config.GetEnvOrDefault("KITE_CLUSTER_URL", "https://konflux.dev")
		logsEndpoint := config.GetEnvOrDefault("KITE_LOGS_ENDPOINT", "/logs/pipelineruns/")
		logsURL = fmt.Sprintf("%s%s%s", baseURL, logsEndpoint, req.RunID)
	}

	severity := models.SeverityMajor
	if req.Severity != "" {
		severity = models.Severity(req.Severity)
	}

	issueData := dto.CreateIssueRequest{
		Title:       fmt.Sprintf("Pipeline run failed: %s", req.PipelineName),
		Description: fmt.Sprintf("The pipeline run %s failed with reason: %s", req.PipelineName, req.FailureReason),
		Severity:    severity,
		IssueType:   models.IssueTypePipeline,
		Namespace:   req.Namespace,
		Scope: dto.ScopeReqBody{
			ResourceType:      "pipelinerun",
			ResourceName:      req.PipelineName,
			ResourceNamespace: req.Namespace,
		},
		Links: []dto.CreateLinkRequest{
			{
				Title: "Pipeline Run Logs",
				URL:   logsURL,
			},
		},
	}

	// Create or update the issue
	issue, err := h.issueService.CreateOrUpdateIssue(c, issueData)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create or update pipeline")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process webhook: %v", err)})
		return
	}

	h.logger.WithField("issue_id", issue.ID).Info("Processed pipeline failure webhook")

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"issue":  issue,
	})
}

// PipelineSuccess handles pipeline success webhooks
func (h *WebhookHandler) PipelineSuccess(c *gin.Context) {
	var req PipelineSuccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required fields", "details": err.Error()})
		return
	}

	// Resolve any active issues for this pipeline
	resolved, err := h.issueService.ResolveIssuesByScope(c.Request.Context(), "pipelinerun", req.PipelineName, req.Namespace)
	if err != nil {
		h.logger.WithError(err).Errorf("failed to resolve issues for pipeline run %s : %v", req.PipelineName, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to resolve issues for pipeline: %v", err),
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"pipeline":  req.PipelineName,
		"namespace": req.Namespace,
		"resolved":  resolved,
	}).Info("Pipeline success webhook processed")

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Resolved %d issue(s) for pipeline %s", resolved, req.PipelineName),
	})
}
