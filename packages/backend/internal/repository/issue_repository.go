package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/konflux-ci/kite/internal/handlers/dto"
	"github.com/konflux-ci/kite/internal/models"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type issueRepository struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// NewIssueRepository creates a new Issue repository
func NewIssueRepository(db *gorm.DB, logger *logrus.Logger) IssueRepository {
	return &issueRepository{
		db:     db,
		logger: logger,
	}
}

func (i *issueRepository) CreateOrUpdate(ctx context.Context, req dto.IssuePayload) (*models.Issue, error) {
	var issue *models.Issue
	var isUpdate bool

	err := i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingIssue *models.Issue
		existingIssue, err := i.findDuplicateInTx(tx, req)

		if err != nil {
			return fmt.Errorf("failed to check for existing issue: %w", err)
		}

		// Create a new one
		if existingIssue == nil {
			newIssue, err := i.createNewIssueInTx(tx, req)
			if err != nil {
				return fmt.Errorf("failed to create issue: %w", err)
			}
			issue = newIssue
			return nil
		}

		// If no error, an existing issue should be found
		isUpdate = true
		issue = existingIssue
		return i.updateIssueInTx(tx, existingIssue, req)
	})

	if err != nil {
		i.logger.WithError(err).Error("Failed to create or update issue")
		return nil, err
	}

	if isUpdate {
		i.logger.WithField("issue_id", issue.ID).Info("Updated existing issue")
	} else {
		i.logger.WithField("issue_id", issue.ID).Info("Created new issue")
	}

	// Reload all associations
	return i.FindByID(ctx, issue.ID)
}

// Checks if an issue that matches the request data already exists.
func (i *issueRepository) FindDuplicate(ctx context.Context, req dto.IssuePayload) (*models.Issue, error) {
	var issue *models.Issue
	err := i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existingIssue, err := i.findDuplicateInTx(tx, req)
		if err != nil {
			i.logger.WithError(err).Error("Failed to check for duplicate issues")
			return err
		}
		if existingIssue != nil {
			i.logger.WithField("existing_issue_id", existingIssue.ID).Info("Found duplicate issue")
			issue = existingIssue
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if issue == nil {
		return nil, nil
	}

	return issue, nil
}

// Private duplicate check implementation
func (i *issueRepository) findDuplicateInTx(tx *gorm.DB, req dto.IssuePayload) (*models.Issue, error) {
	var existingIssue models.Issue
	// Try to find an existing issue matching these values.
	// Do it all in the same DB transaction and use "FOR UPDATE"
	// to lock the rows and prevent potential race conditions.
	// Doc: https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-ROWS
	err := tx.Preload("Links").
		Joins("JOIN issue_scopes on issues.scope_id = issue_scopes.id").
		Where("issues.namespace = ? AND issues.issue_type = ? AND issues.state = ?",
			req.GetNamespace(), req.GetIssueType(), models.IssueStateActive).
		Where("issue_scopes.resource_type = ? AND issue_scopes.resource_name = ? AND issue_scopes.resource_namespace = ?",
			req.GetScope().ResourceType, req.GetScope().ResourceName, req.GetNamespace()).
		Set("gorm:query_option", "FOR UPDATE").
		First(&existingIssue).Error

	if err != nil {
		// Check if the error is no record was found.
		// If it is, the issue is not a duplicate.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to check for duplicates: %w", err)
	}
	return &existingIssue, nil
}

type IssueQueryFilters struct {
	Namespace    string
	Severity     *models.Severity
	IssueType    *models.IssueType
	State        *models.IssueState
	ResourceType string
	ResourceName string
	Search       string
	Limit        int
	Offset       int
}

func (i *issueRepository) FindAll(ctx context.Context, filters IssueQueryFilters) ([]models.Issue, int64, error) {
	var issues []models.Issue
	var total int64

	// Build base query
	// Preload any associations
	query := i.db.WithContext(ctx).Model(&models.Issue{}).
		Preload("Scope").
		Preload("Links").
		Preload("RelatedFrom.Target.Scope").
		Preload("RelatedTo.Source.Scope")

	// Apply filters to the database query
	if filters.Namespace != "" {
		query = query.Where("namespace = ?", filters.Namespace)
	}
	if filters.Severity != nil {
		query = query.Where("severity = ?", *filters.Severity)
	}
	if filters.IssueType != nil {
		query = query.Where("issue_type = ?", *filters.IssueType)
	}
	if filters.State != nil {
		query = query.Where("state = ?", *filters.State)
	}
	if filters.ResourceType != "" {
		query = query.Joins("JOIN issue_scopes ON issues.scope_id = issue_scopes.id").
			Where("issue_scopes.resource_type = ?", filters.ResourceType)
	}
	if filters.ResourceName != "" {
		query = query.Joins("JOIN issue_scopes ON issues.scope_id = issue_scopes.id").
			Where("issue_scopes.resource_name = ?", filters.ResourceName)
	}
	if filters.Search != "" {
		searchPattern := "%" + filters.Search + "%"
		// Use LIKE instead of ILIKE for portability.
		// Use LOWER to prevent any case sensitivity issues
		query = query.Where("LOWER(title) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?)", searchPattern, searchPattern)
	}

	// Get total count for pagination
	if err := query.Count(&total).Error; err != nil {
		i.logger.WithError(err).Error("Failed to count issues")
		return nil, 0, fmt.Errorf("failed to count issues: %w", err)
	}

	// Apply pagination and ordering
	if filters.Limit == 0 {
		filters.Limit = 50
	}

	if err := query.Order("detected_at DESC").
		Offset(filters.Offset).
		Limit(filters.Limit).
		Find(&issues).
		Error; err != nil {
		i.logger.WithError(err).Error("Failed to find issues")
		return nil, 0, fmt.Errorf("failed to find issues: %w", err)
	}

	return issues, total, nil
}

func (i *issueRepository) FindByID(ctx context.Context, id string) (*models.Issue, error) {
	var issue models.Issue

	// Find issue, load associations
	err := i.db.
		WithContext(ctx).
		Preload("Scope").
		Preload("RelatedFrom.Target.Scope").
		Preload("RelatedTo.Source.Scope").
		First(&issue, "id = ?", id).Error

	if err != nil {
		// Check if the error is record not found
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		i.logger.WithError(err).WithField("issue_id", id).Error("failed to find issue by ID")
		return nil, fmt.Errorf("failed to find issue: %w", err)
	}
	return &issue, nil
}

// Creates an Issue record
func (i *issueRepository) Create(ctx context.Context, req dto.IssuePayload) (*models.Issue, error) {
	var issue *models.Issue
	// check for duplicates before creating.
	err := i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existingIssue, err := i.findDuplicateInTx(tx, req)
		if err != nil {
			return fmt.Errorf("failed to check for duplicates: %w", err)
		}

		if existingIssue != nil {
			// Update existing issue instead of creating a new one
			updateReq := dto.UpdateIssueRequest{
				Title:       req.GetTitle(),
				Description: req.GetDescription(),
				Severity:    req.GetSeverity(),
				IssueType:   req.GetIssueType(),
				Scope:       req.GetScope(),
				Namespace:   req.GetNamespace(),
				State:       req.GetState(),
			}
			issue = existingIssue
			return i.updateIssueInTx(tx, existingIssue, updateReq)
		}

		newIssue, err := i.createNewIssueInTx(tx, req)
		if err != nil {
			return err
		}

		issue = newIssue
		return nil
	})

	if err != nil {
		return nil, err
	}

	if issue == nil {
		i.logger.WithField("request", req).Error("Failed to create an issue: no issue returned")
		return nil, errors.New("issue creation failed: no issue returned")
	}

	i.logger.WithField("issue_id", issue.ID).Info("Created new issue")

	// Reload with associations
	return i.FindByID(ctx, issue.ID)
}

// Private implementation for creating issues in a Database transaction
func (i *issueRepository) createNewIssueInTx(tx *gorm.DB, req dto.IssuePayload) (*models.Issue, error) {
	now := time.Now()
	state := req.GetState()
	if state == "" {
		state = models.IssueStateActive
	}

	resourceNamespace := req.GetScope().ResourceNamespace
	if resourceNamespace == "" {
		resourceNamespace = req.GetNamespace()
	}

	newIssue := &models.Issue{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Severity:    req.GetSeverity(),
		IssueType:   req.GetIssueType(),
		State:       state,
		DetectedAt:  now,
		Namespace:   req.GetNamespace(),
		Scope: models.IssueScope{
			ResourceType:      req.GetScope().ResourceType,
			ResourceName:      req.GetScope().ResourceName,
			ResourceNamespace: resourceNamespace,
		},
	}

	// Convert links
	for _, linkReq := range req.GetLinks() {
		newIssue.Links = append(newIssue.Links, models.Link{
			Title: linkReq.Title,
			URL:   linkReq.URL,
		})
	}

	if err := tx.Create(&newIssue).Error; err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}

	return newIssue, nil
}

// Updates an existing issue record
func (i *issueRepository) Update(ctx context.Context, id string, req dto.IssuePayload) (*models.Issue, error) {
	// Find existing issue
	existingIssue, err := i.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existingIssue == nil {
		return nil, fmt.Errorf("issue with ID %s not found", id)
	}

	err = i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return i.updateIssueInTx(tx, existingIssue, req)
	})

	if err != nil {
		i.logger.WithError(err).WithField("issue_id", id).Error("Failed to update issue")
		return nil, err
	}

	i.logger.WithField("issue_id", id).Info("Updated issue")

	return i.FindByID(ctx, id)
}

// Private implementation for updating issues in a Database transaction
func (i *issueRepository) updateIssueInTx(tx *gorm.DB, existingIssue *models.Issue, req dto.IssuePayload) error {
	// Prepare updates
	updates := map[string]any{
		"title":       req.GetTitle(),
		"description": req.GetDescription(),
		"severity":    req.GetSeverity(),
		"issue_type":  req.GetIssueType(),
		"namespace":   req.GetNamespace(),
		"updated_at":  time.Now(),
	}

	if req.GetState() != "" {
		updates["state"] = req.GetState()
		if req.GetState() == models.IssueStateResolved && existingIssue.State != models.IssueStateResolved {
			updates["resolved_at"] = time.Now()
		} else if ra := req.GetResolvedAt(); !ra.IsZero() {
			updates["resolved_at"] = ra
		}
	}

	// Update the issue
	if err := tx.Model(existingIssue).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}

	// Handle link updates if provided
	if links := req.GetLinks(); len(links) > 0 {
		err := i.replaceIssueLinks(tx, existingIssue.ID, links)
		if err != nil {
			return fmt.Errorf("failed to replace links for issue: %w", err)
		}
		i.logger.WithField("issue_id", existingIssue.ID).Info("Updated links")
	}

	if scope := req.GetScope(); scope.ResourceNamespace != "" {
		err := tx.Model(&models.IssueScope{}).
			Where("id = ?", existingIssue.ScopeID).
			Updates(scope).Error

		if err != nil {
			return fmt.Errorf("failed to update issue scope")
		}
		i.logger.WithField("issue_id", existingIssue.ID).Info("Updated scope")
	}

	return nil
}

func (i *issueRepository) replaceIssueLinks(tx *gorm.DB, issueID string, links []dto.CreateLinkRequest) error {
	// Delete old links
	if err := tx.Where("issue_id = ?", issueID).Delete(&models.Link{}).Error; err != nil {
		return fmt.Errorf("failed to delete old links: %w", err)
	}

	// Create new links
	for _, linkReq := range links {
		link := models.Link{
			Title:   linkReq.Title,
			URL:     linkReq.URL,
			IssueID: issueID,
		}
		if err := tx.Create(&link).Error; err != nil {
			return fmt.Errorf("failed to create link: %w", err)
		}
	}
	return nil
}

func (i *issueRepository) Delete(ctx context.Context, id string) error {
	// Find the issue to get scope ID
	issue, err := i.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("issue with ID %s not found", id)
	}

	// Delete in transaction so we have control of the order
	err = i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete related issue relationships first using issue id
		if err := tx.Where("source_id = ? OR target_id = ?", id, id).Delete(&models.RelatedIssue{}).Error; err != nil {
			return fmt.Errorf("failed to delete related issues: %w", err)
		}

		// Delete links by issue id
		if err := tx.Where("issue_id = ?", id).Delete(&models.Link{}).Error; err != nil {
			return fmt.Errorf("failed to delete links: %w", err)
		}

		// Delete the issue by id
		if err := tx.Delete(&models.Issue{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("failed to delete issue: %w", err)
		}

		// Delete the issue scope by scope id
		if err := tx.Delete(&models.IssueScope{}, "id = ?", issue.ScopeID).Error; err != nil {
			return fmt.Errorf("failed to delete issue scope: %w", err)
		}

		return nil
	})

	if err != nil {
		i.logger.WithError(err).WithField("issue_id", id).Error("failed to delete issue")
		return err
	}

	i.logger.WithField("issue_id", id).Info("Deleted issue")
	return nil
}

func (i *issueRepository) ResolveByScope(ctx context.Context, resourceType, resourceName, namespace string) (int64, error) {
	now := time.Now()

	// Get the IDs of all issues meeting this criteria
	var ids []string
	query := i.db.WithContext(ctx).Model(&models.Issue{}).
		Joins("JOIN issue_scopes ON issues.scope_id = issue_scopes.id").
		Where("issues.state = ? AND issues.namespace = ?", models.IssueStateActive, namespace).
		Where("issue_scopes.resource_type = ? AND issue_scopes.resource_name = ?", resourceType, resourceName).
		Pluck("issues.id", &ids)

	// Check for error in query
	if query.Error != nil {
		return 0, fmt.Errorf("failed to query issue IDs to resolve: %w", query.Error)
	}

	// Check if any issues were found
	if len(ids) == 0 {
		i.logger.WithFields(logrus.Fields{
			"resource_type": resourceType,
			"resource_name": resourceName,
			"namespace":     namespace,
		}).Info("No active issues found for scope")
		return 0, nil
	}

	// Update issues by ID
	result := i.db.
		WithContext(ctx).
		Model(&models.Issue{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"state":       models.IssueStateResolved,
			"resolved_at": &now,
			"updated_at":  now,
		})

	if result.Error != nil {
		i.logger.WithError(result.Error).Error("Failed to resolve issues by scope")
		return 0, fmt.Errorf("failed to resolve issues: %w", result.Error)
	}

	count := result.RowsAffected
	i.logger.WithFields(logrus.Fields{
		"resource_type": resourceType,
		"resource_name": resourceName,
		"namespace":     namespace,
		"count":         count,
	}).Info("Resolved issues by scope")

	return count, nil
}

// AddRelatedIsue creates a relationship between two issues
func (i *issueRepository) AddRelatedIssue(ctx context.Context, sourceID, targetID string) error {
	// Check if both issues exist
	source, err := i.FindByID(ctx, sourceID)
	if err != nil {
		return err
	}
	target, err := i.FindByID(ctx, targetID)
	if err != nil {
		return err
	}
	if source == nil || target == nil {
		return errors.New("one or both issues not found")
	}

	// Check if relationship already exists
	var existingRelation models.RelatedIssue
	err = i.db.WithContext(ctx).Where("(source_id = ? AND target_id = ?) OR (source_id = ? AND target_id = ?)",
		sourceID, targetID, targetID, sourceID).First(&existingRelation).Error

	if err == nil {
		return errors.New("relationship already exists")
	}
	// Check if we get any other error besides Record Not Found
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check exiting relationship: %w", err)
	}

	// Create relationship
	relation := models.RelatedIssue{
		SourceID: sourceID,
		TargetID: targetID,
	}

	if err := i.db.WithContext(ctx).Create(&relation).Error; err != nil {
		i.logger.WithError(err).Error("Failed to add related issue")
		return fmt.Errorf("failed to create relationship: %w", err)
	}

	i.logger.WithFields(logrus.Fields{
		"source_id": sourceID,
		"target_id": targetID,
	}).Info("Added related issue")
	return nil
}

// RemoveRelatedIssue removes a relationship between issues
func (i *issueRepository) RemoveRelatedIssue(ctx context.Context, sourceID, targetID string) error {
	result := i.db.WithContext(ctx).Where("(source_id = ? AND target_id = ?) OR (source_id = ? AND target_id = ?)",
		sourceID, targetID, targetID, sourceID).Delete(&models.RelatedIssue{})

	if result.Error != nil {
		i.logger.WithError(result.Error).Error("failed to remove related issue")
		return fmt.Errorf("failed to remove relationship: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.New("relationship not found")
	}

	i.logger.WithFields(logrus.Fields{
		"source_id": sourceID,
		"target_id": targetID,
	}).Info("Removed related issue")

	return nil
}
