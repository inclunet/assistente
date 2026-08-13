package portability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

func exportTaskListWithContext(ctx context.Context, taskListID string) (TaskListExport, error) {
	taskList, err := database.GetTaskListWithContext(ctx, taskListID)
	if err != nil {
		return TaskListExport{}, err
	}
	if taskList.Workflow == nil {
		return TaskListExport{}, fmt.Errorf("tasklist %s sem workflow", taskListID)
	}

	db := database.DB()

	var allTasks []database.Task
	if err := db.Where("task_list_id = ?", taskListID).
		Order("parent_id ASC, `order` ASC, created_at ASC").
		Find(&allTasks).Error; err != nil {
		return TaskListExport{}, err
	}

	taskIDs := make([]string, 0, len(allTasks))
	for _, task := range allTasks {
		taskIDs = append(taskIDs, task.ID)
	}

	notesByTaskID := make(map[string][]database.TaskNote, len(taskIDs))
	if len(taskIDs) > 0 {
		var notes []database.TaskNote
		if err := db.Where("task_id IN ?", taskIDs).
			Order("created_at ASC").
			Find(&notes).Error; err != nil {
			return TaskListExport{}, err
		}
		for _, note := range notes {
			notesByTaskID[note.TaskID] = append(notesByTaskID[note.TaskID], note)
		}
	}

	workflow, err := exportTaskListWorkflow(*taskList.Workflow)
	if err != nil {
		return TaskListExport{}, err
	}

	childrenByParentID := make(map[string][]database.Task)
	rootTasks := make([]database.Task, 0)
	for _, task := range allTasks {
		if task.ParentID == nil {
			rootTasks = append(rootTasks, task)
			continue
		}
		childrenByParentID[*task.ParentID] = append(childrenByParentID[*task.ParentID], task)
	}

	exportedTasks := make([]TaskExport, 0, len(rootTasks))
	for _, task := range rootTasks {
		exportedTasks = append(exportedTasks, exportTaskNode(task, childrenByParentID, notesByTaskID))
	}

	return TaskListExport{
		ID:                taskList.ID,
		Title:             taskList.Title,
		Slug:              strings.TrimSpace(taskList.Slug),
		Description:       taskList.Description,
		PreferredViewMode: taskList.PreferredViewMode,
		ValidationPolicy:  strings.TrimSpace(taskList.ValidationPolicy),
		CreatedAt:         taskList.CreatedAt,
		Workflow:          workflow,
		Tasks:             exportedTasks,
	}, nil
}

func exportTaskListWorkflow(workflow database.TaskListWorkflow) (TaskListWorkflowExport, error) {
	var statuses []database.TaskListWorkflowStatus
	if strings.TrimSpace(workflow.Statuses) != "" {
		if err := json.Unmarshal([]byte(workflow.Statuses), &statuses); err != nil {
			return TaskListWorkflowExport{}, fmt.Errorf("erro ao ler statuses do workflow: %w", err)
		}
	}

	transitions := make(map[int][]int)
	if strings.TrimSpace(workflow.AllowedTransitions) != "" {
		if err := json.Unmarshal([]byte(workflow.AllowedTransitions), &transitions); err != nil {
			return TaskListWorkflowExport{}, fmt.Errorf("erro ao ler transições do workflow: %w", err)
		}
	}

	exportedStatuses := make([]TaskListWorkflowStatusExport, 0, len(statuses))
	for _, status := range statuses {
		exportedStatuses = append(exportedStatuses, TaskListWorkflowStatusExport{
			ID:    status.ID,
			Order: status.Order,
			Label: status.Label,
			Color: status.Color,
			Icon:  status.Icon,
		})
	}

	sort.Slice(exportedStatuses, func(i, j int) bool {
		if exportedStatuses[i].Order == exportedStatuses[j].Order {
			return exportedStatuses[i].ID < exportedStatuses[j].ID
		}
		return exportedStatuses[i].Order < exportedStatuses[j].Order
	})

	return TaskListWorkflowExport{
		ID:                 workflow.ID,
		TaskListID:         workflow.TaskListID,
		Statuses:           exportedStatuses,
		AllowedTransitions: transitions,
		InitialStatusID:    workflow.InitialStatusID,
	}, nil
}

func exportTaskNode(
	task database.Task,
	childrenByParentID map[string][]database.Task,
	notesByTaskID map[string][]database.TaskNote,
) TaskExport {
	exportedNotes := make([]TaskNoteExport, 0, len(notesByTaskID[task.ID]))
	for _, note := range notesByTaskID[task.ID] {
		exportedNotes = append(exportedNotes, TaskNoteExport{
			ID:                note.ID,
			TaskID:            note.TaskID,
			Type:              int(note.Type),
			Content:           note.Content,
			AuthorName:        note.AuthorName,
			AuthorID:          note.AuthorID,
			Source:            note.ExternalSource,
			ExternalID:        note.ExternalID,
			ExternalParentID:  note.ExternalParentID,
			ExternalUpdatedAt: note.ExternalUpdatedAt,
			CreatedAt:         note.CreatedAt,
		})
	}

	children := childrenByParentID[task.ID]
	exportedChildren := make([]TaskExport, 0, len(children))
	for _, child := range children {
		exportedChildren = append(exportedChildren, exportTaskNode(child, childrenByParentID, notesByTaskID))
	}

	return TaskExport{
		ID:           task.ID,
		TaskListID:   task.TaskListID,
		ParentID:     derefString(task.ParentID),
		Title:        task.Title,
		Description:  task.Description,
		Code:         task.Code,
		Link:         task.Link,
		StatusID:     task.StatusID,
		Order:        task.Order,
		AssigneeName: task.AssigneeName,
		AssigneeID:   task.AssigneeID,
		CreatorName:  task.CreatorName,
		CreatorID:    task.CreatorID,
		DueDate:      task.DueDate,
		CompletedAt:  task.CompletedAt,
		CreatedAt:    task.CreatedAt,
		Notes:        exportedNotes,
		Children:     exportedChildren,
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func importTaskList(ctx context.Context, taskList TaskListExport) (bool, error) {
	if existing, err := findExistingTaskListByExport(ctx, taskList); err != nil {
		return false, err
	} else if existing != nil {
		return overwriteTaskList(ctx, taskList)
	}

	err := database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return persistTaskList(ctx, tx, taskList, nil)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func overwriteTaskList(ctx context.Context, taskList TaskListExport) (bool, error) {
	existing, err := findExistingTaskListByExport(ctx, taskList)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return importTaskList(ctx, taskList)
	}

	err = database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return persistTaskList(ctx, tx, taskList, existing)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func persistTaskList(ctx context.Context, tx *gorm.DB, taskList TaskListExport, existing *database.TaskList) error {
	taskListID := strings.TrimSpace(taskList.ID)
	if taskListID == "" {
		return codedErrorf(
			CodeTaskListMissingID,
			params("taskList", taskList.Title, "version", itoa(ExportVersion)),
			"tasklist %q sem id não pode ser importada no formato version %d", taskList.Title, ExportVersion,
		)
	}
	workflowID := strings.TrimSpace(taskList.Workflow.ID)
	if workflowID == "" {
		return codedErrorf(
			CodeTaskListWorkflowMissingID,
			params("taskList", taskList.Title, "version", itoa(ExportVersion)),
			"workflow da tasklist %q sem id não pode ser importado no formato version %d", taskList.Title, ExportVersion,
		)
	}

	workflowStatuses, workflowTransitions, err := validateImportedTaskListWorkflow(taskList.Workflow)
	if err != nil {
		return err
	}

	taskStatusIDs := make(map[int]struct{}, len(workflowStatuses))
	for _, status := range workflowStatuses {
		taskStatusIDs[status.ID] = struct{}{}
	}

	viewMode := strings.TrimSpace(taskList.PreferredViewMode)
	if viewMode != "kanban" {
		viewMode = "list"
	}

	createdAt := taskList.CreatedAt
	if createdAt.IsZero() {
		if existing != nil && !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		} else {
			createdAt = time.Now().UTC()
		}
	}
	updatedAt := createdAt
	if existing != nil && taskList.CreatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	model := database.TaskList{
		UUIDModel: database.UUIDModel{
			ID:        taskListID,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Title:             taskList.Title,
		Slug:              database.NormalizeTaskListSlug(taskList.Slug),
		Description:       taskList.Description,
		PreferredViewMode: viewMode,
		ValidationPolicy:  strings.TrimSpace(taskList.ValidationPolicy),
	}
	if userID, ok := database.UserIDFromContext(ctx); ok {
		model.UserID = userID
	}
	if existing == nil {
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
	} else {
		existing.Title = model.Title
		existing.Slug = model.Slug
		existing.Description = model.Description
		existing.PreferredViewMode = model.PreferredViewMode
		existing.ValidationPolicy = model.ValidationPolicy
		existing.CreatedAt = model.CreatedAt
		existing.UpdatedAt = model.UpdatedAt
		if err := tx.Save(existing).Error; err != nil {
			return err
		}
		model.ID = existing.ID

		var taskIDs []string
		if err := tx.Model(&database.Task{}).Where("task_list_id = ?", model.ID).Pluck("id", &taskIDs).Error; err != nil {
			return err
		}
		if len(taskIDs) > 0 {
			if err := tx.Where("task_id IN ?", taskIDs).Delete(&database.TaskNote{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("task_list_id = ?", model.ID).Delete(&database.Task{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_list_id = ?", model.ID).Delete(&database.TaskListWorkflow{}).Error; err != nil {
			return err
		}
	}

	statusesJSON, err := json.Marshal(workflowStatuses)
	if err != nil {
		return err
	}
	transitionsJSON, err := json.Marshal(workflowTransitions)
	if err != nil {
		return err
	}

	workflow := database.TaskListWorkflow{
		UUIDModel: database.UUIDModel{
			ID:        workflowID,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		TaskListID:         model.ID,
		Statuses:           string(statusesJSON),
		AllowedTransitions: string(transitionsJSON),
		InitialStatusID:    taskList.Workflow.InitialStatusID,
	}
	if err := tx.Create(&workflow).Error; err != nil {
		return err
	}

	for _, task := range taskList.Tasks {
		if err := importTaskNode(tx, model.ID, nil, task, taskStatusIDs); err != nil {
			return err
		}
	}

	return nil
}

func validateImportedTaskListWorkflow(workflow TaskListWorkflowExport) ([]database.TaskListWorkflowStatus, map[int][]int, error) {
	if len(workflow.Statuses) == 0 {
		return nil, nil, codedErrorf(
			CodeTaskListWorkflowWithoutStatuses, nil,
			"workflow da tasklist deve ter ao menos um status",
		)
	}

	statusIDs := make(map[int]struct{}, len(workflow.Statuses))
	convertedStatuses := make([]database.TaskListWorkflowStatus, 0, len(workflow.Statuses))
	for _, status := range workflow.Statuses {
		if status.ID <= 0 {
			return nil, nil, codedErrorf(
				CodeTaskListWorkflowInvalidStatus,
				params("statusId", itoa(status.ID)),
				"workflow da tasklist contém status inválido: %d", status.ID,
			)
		}
		if _, exists := statusIDs[status.ID]; exists {
			return nil, nil, codedErrorf(
				CodeTaskListWorkflowDuplicatedStatus,
				params("statusId", itoa(status.ID)),
				"workflow da tasklist contém status duplicado: %d", status.ID,
			)
		}
		statusIDs[status.ID] = struct{}{}
		convertedStatuses = append(convertedStatuses, database.TaskListWorkflowStatus{
			ID:    status.ID,
			Order: status.Order,
			Label: status.Label,
			Color: status.Color,
			Icon:  status.Icon,
		})
	}

	if _, exists := statusIDs[workflow.InitialStatusID]; !exists {
		return nil, nil, codedErrorf(
			CodeTaskListWorkflowInitialUnknown,
			params("statusId", itoa(workflow.InitialStatusID)),
			"initialStatusId %d não existe no workflow da tasklist", workflow.InitialStatusID,
		)
	}

	convertedTransitions := make(map[int][]int, len(workflow.AllowedTransitions))
	for fromID, toIDs := range workflow.AllowedTransitions {
		if _, exists := statusIDs[fromID]; !exists {
			return nil, nil, codedErrorf(
				CodeTaskListWorkflowFromUnknown,
				params("statusId", itoa(fromID)),
				"workflow referencia status de origem inexistente: %d", fromID,
			)
		}
		copied := append([]int(nil), toIDs...)
		for _, toID := range copied {
			if _, exists := statusIDs[toID]; !exists {
				return nil, nil, codedErrorf(
					CodeTaskListWorkflowToUnknown,
					params("statusId", itoa(toID)),
					"workflow referencia status de destino inexistente: %d", toID,
				)
			}
		}
		convertedTransitions[fromID] = copied
	}

	return convertedStatuses, convertedTransitions, nil
}

func importTaskNode(
	tx *gorm.DB,
	taskListID string,
	parentID *string,
	task TaskExport,
	validStatusIDs map[int]struct{},
) error {
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		return codedErrorf(
			CodeTaskMissingID,
			params("task", task.Title, "version", itoa(ExportVersion)),
			"task %q sem id não pode ser importada no formato version %d", task.Title, ExportVersion,
		)
	}
	if _, exists := validStatusIDs[task.StatusID]; !exists {
		return codedErrorf(
			CodeTaskUnknownStatus,
			params("task", task.Title, "statusId", itoa(task.StatusID)),
			"task %q referencia status inexistente: %d", task.Title, task.StatusID,
		)
	}

	createdAt := task.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	model := database.Task{
		UUIDModel: database.UUIDModel{
			ID:        taskID,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		TaskListID:   taskListID,
		Title:        task.Title,
		Description:  task.Description,
		Code:         task.Code,
		Link:         task.Link,
		StatusID:     task.StatusID,
		ParentID:     parentID,
		Order:        task.Order,
		AssigneeName: task.AssigneeName,
		AssigneeID:   task.AssigneeID,
		CreatorName:  task.CreatorName,
		CreatorID:    task.CreatorID,
		DueDate:      task.DueDate,
		CompletedAt:  task.CompletedAt,
	}
	if err := tx.Create(&model).Error; err != nil {
		return err
	}

	for _, note := range task.Notes {
		noteID := strings.TrimSpace(note.ID)
		if noteID == "" {
			return codedErrorf(
				CodeTaskNoteMissingID,
				params("task", task.Title, "version", itoa(ExportVersion)),
				"nota da task %q sem id não pode ser importada no formato version %d", task.Title, ExportVersion,
			)
		}
		noteCreatedAt := note.CreatedAt
		if noteCreatedAt.IsZero() {
			noteCreatedAt = createdAt
		}
		noteModel := database.TaskNote{
			UUIDModel: database.UUIDModel{
				ID:        noteID,
				CreatedAt: noteCreatedAt,
				UpdatedAt: noteCreatedAt,
			},
			TaskID:            model.ID,
			Type:              database.TaskNoteType(note.Type),
			Content:           note.Content,
			AuthorName:        note.AuthorName,
			AuthorID:          note.AuthorID,
			ExternalSource:    note.Source,
			ExternalID:        note.ExternalID,
			ExternalParentID:  note.ExternalParentID,
			ExternalUpdatedAt: note.ExternalUpdatedAt,
		}
		if err := tx.Create(&noteModel).Error; err != nil {
			return err
		}
	}

	for _, child := range task.Children {
		parent := model.ID
		if err := importTaskNode(tx, taskListID, &parent, child, validStatusIDs); err != nil {
			return err
		}
	}

	return nil
}

func findExistingTaskListByExport(ctx context.Context, taskList TaskListExport) (*database.TaskList, error) {
	if id := strings.TrimSpace(taskList.ID); id != "" {
		var existing database.TaskList
		err := database.ScopeByUser(ctx, database.DB(), "user_id").Where("id = ?", id).First(&existing).Error
		if err == nil {
			return &existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("erro ao localizar tasklist %q: %w", id, err)
		}
	}
	return nil, nil
}

func countExportedTasks(taskLists []TaskListExport) (int, int) {
	taskCount := 0
	noteCount := 0
	for _, taskList := range taskLists {
		for _, task := range taskList.Tasks {
			tasks, notes := countTaskTree(task)
			taskCount += tasks
			noteCount += notes
		}
	}
	return taskCount, noteCount
}

func countTaskTree(task TaskExport) (int, int) {
	taskCount := 1
	noteCount := len(task.Notes)
	for _, child := range task.Children {
		childTasks, childNotes := countTaskTree(child)
		taskCount += childTasks
		noteCount += childNotes
	}
	return taskCount, noteCount
}
