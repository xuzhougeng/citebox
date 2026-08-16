package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"html"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/zotero"
)

const (
	zoteroItemImported        = "imported"
	zoteroItemSkippedExisting = "skipped_existing"
	zoteroItemMissingPDF      = "missing_pdf"
	zoteroItemError           = "error"
	zoteroRunQueued           = "queued"
	zoteroRunRunning          = "running"
	zoteroRunCompleted        = "completed"
	zoteroRunFailed           = "failed"
)

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

type zoteroCatalog struct {
	LibraryPrefix string
	Collections   []zotero.Collection
	Items         []zotero.Item
	PathByKey     map[string]string
	ByKey         map[string]zotero.Item
}

func (s *LibraryService) zoteroClientFromSettings() (*zotero.Client, *model.ZoteroSettings, error) {
	settings, err := s.GetZoteroSettings()
	if err != nil {
		return nil, nil, err
	}
	client, err := zotero.NewClient(settings.BaseURL)
	if err != nil {
		return nil, nil, apperr.New(apperr.CodeInvalidArgument, err.Error())
	}
	return client, settings, nil
}

func (s *LibraryService) GetZoteroStatus(ctx context.Context) (*model.ZoteroStatus, error) {
	client, settings, err := s.zoteroClientFromSettings()
	if err != nil {
		return nil, err
	}
	status := &model.ZoteroStatus{BaseURL: settings.BaseURL}
	probed, err := client.Probe(ctx)
	if err != nil {
		status.Message = err.Error()
		return status, nil
	}
	status.Reachable = true
	status.LibraryPrefix = probed.LibraryPrefix
	status.CollectionCount = probed.CollectionCount
	status.Message = "已连接到 Zotero Local API"
	return status, nil
}

func (s *LibraryService) ListZoteroCollections(ctx context.Context) ([]model.ZoteroCollectionNode, error) {
	catalog, err := s.loadZoteroCatalog(ctx, false)
	if err != nil {
		return nil, err
	}
	return buildZoteroCollectionTree(catalog.Collections, catalog.PathByKey), nil
}

func (s *LibraryService) PreviewZoteroImport(ctx context.Context, collectionKeys []string, includeChildren bool) (*model.ZoteroImportRun, error) {
	items, err := s.planZoteroImport(ctx, collectionKeys, includeChildren)
	if err != nil {
		return nil, err
	}
	run := &model.ZoteroImportRun{
		Status:          "preview",
		IncludeChildren: includeChildren,
		CollectionKeys:  uniqueStrings(collectionKeys),
		Items:           items,
		Summary:         summarizeZoteroItems(items),
	}
	return run, nil
}

func (s *LibraryService) StartZoteroImport(ctx context.Context, collectionKeys []string, includeChildren bool) (*model.ZoteroImportRun, error) {
	running, err := s.repo.ZoteroImport.HasRunning()
	if err != nil {
		return nil, err
	}
	if running {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "已有 Zotero 导入正在进行")
	}
	items, err := s.planZoteroImport(ctx, collectionKeys, includeChildren)
	if err != nil {
		return nil, err
	}
	run := &model.ZoteroImportRun{
		ID:              newZoteroRunID(),
		Status:          zoteroRunQueued,
		IncludeChildren: includeChildren,
		CollectionKeys:  uniqueStrings(collectionKeys),
		Items:           items,
		Summary:         summarizeZoteroItems(items),
	}
	if err := s.repo.ZoteroImport.Save(run); err != nil {
		return nil, err
	}
	if settings, err := s.GetZoteroSettings(); err == nil {
		settings.IncludeChildren = includeChildren
		settings.LastCollectionKeys = run.CollectionKeys
		settings.LastRunID = run.ID
		_, _ = s.UpdateZoteroSettings(*settings)
	}
	go s.executeZoteroImport(run.ID)
	return run, nil
}

func (s *LibraryService) GetZoteroImportRun(id string) (*model.ZoteroImportRun, error) {
	run, err := s.repo.ZoteroImport.Get(strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, apperr.New(apperr.CodeNotFound, "找不到该 Zotero 导入任务")
	}
	return run, nil
}

func (s *LibraryService) IngestZoteroPaperFromReader(file multipart.File, header *multipart.FileHeader, input model.ZoteroIngestInput) (*model.Paper, string, error) {
	if file == nil || header == nil {
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "缺少 PDF 文件")
	}
	item := plannedItemFromInput(input)
	item.HasLocalPDF = true
	item.PDFFilename = header.Filename
	paper, status, err := s.ingestZoteroItem(item, func(params UploadPaperParams) (*model.Paper, error) {
		params.ExtractionMode = input.ExtractionMode
		return s.UploadPaper(file, header, params)
	})
	if err != nil {
		return nil, "", err
	}
	return paper, status, nil
}

func (s *LibraryService) AttachZoteroImportPDF(runID, itemKey string, file multipart.File, header *multipart.FileHeader) (*model.ZoteroImportRun, error) {
	run, item, index, err := s.lookupZoteroRunItem(runID, itemKey)
	if err != nil {
		return nil, err
	}
	if file == nil || header == nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少 PDF 文件")
	}
	item.HasLocalPDF = true
	item.PDFFilename = header.Filename
	paper, status, err := s.ingestZoteroItem(item, func(params UploadPaperParams) (*model.Paper, error) {
		return s.UploadPaper(file, header, params)
	})
	if err != nil {
		item.Status = zoteroItemError
		item.Reason = err.Error()
		run.Items[index] = item
		run.Summary = summarizeZoteroItems(run.Items)
		_ = s.repo.ZoteroImport.Save(run)
		return nil, err
	}
	if paper != nil {
		item.PaperID = paper.ID
	}
	item.Status = status
	item.Reason = statusReason(status)
	run.Items[index] = item
	run.Summary = summarizeZoteroItems(run.Items)
	if err := s.repo.ZoteroImport.Save(run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *LibraryService) ImportZoteroItemByDOI(ctx context.Context, runID, itemKey string) (*model.ZoteroImportRun, error) {
	run, item, index, err := s.lookupZoteroRunItem(runID, itemKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(item.DOI) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "该条目没有 DOI，请改为上传本地 PDF")
	}
	groupID, err := s.groupIDForZoteroPath(item.CollectionPath)
	if err != nil {
		return nil, err
	}
	paper, err := s.ImportPaperByDOI(ctx, ImportPaperByDOIParams{
		DOI:     item.DOI,
		Title:   item.Title,
		GroupID: groupID,
		Tags:    item.Tags,
	})
	status := zoteroItemImported
	if err != nil {
		var duplicateErr *DuplicatePaperError
		if !errors.As(err, &duplicateErr) || duplicateErr.Paper == nil {
			item.Status = zoteroItemError
			item.Reason = err.Error()
			run.Items[index] = item
			run.Summary = summarizeZoteroItems(run.Items)
			_ = s.repo.ZoteroImport.Save(run)
			return nil, err
		}
		paper = duplicateErr.Paper
		status = zoteroItemSkippedExisting
	}
	if err := s.bindZoteroExternalID(paper.ID, item); err != nil {
		return nil, err
	}
	if status == zoteroItemImported {
		if updated, updateErr := s.applyZoteroNotes(paper, item.NotesText); updateErr == nil {
			paper = updated
		}
	}
	item.PaperID = paper.ID
	item.Status = status
	item.Reason = statusReason(status)
	run.Items[index] = item
	run.Summary = summarizeZoteroItems(run.Items)
	if err := s.repo.ZoteroImport.Save(run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *LibraryService) lookupZoteroRunItem(runID, itemKey string) (*model.ZoteroImportRun, model.ZoteroImportItem, int, error) {
	run, err := s.GetZoteroImportRun(runID)
	if err != nil {
		return nil, model.ZoteroImportItem{}, -1, err
	}
	itemKey = strings.TrimSpace(itemKey)
	for i, item := range run.Items {
		if item.ItemKey == itemKey {
			return run, item, i, nil
		}
	}
	return nil, model.ZoteroImportItem{}, -1, apperr.New(apperr.CodeNotFound, "导入任务中找不到该 Zotero 条目")
}

func (s *LibraryService) executeZoteroImport(runID string) {
	run, err := s.repo.ZoteroImport.Get(runID)
	if err != nil || run == nil {
		return
	}
	run.Status = zoteroRunRunning
	if err := s.repo.ZoteroImport.Save(run); err != nil {
		return
	}
	for i := range run.Items {
		item := run.Items[i]
		if !item.HasLocalPDF || strings.TrimSpace(item.PDFPath) == "" {
			item.Status = zoteroItemMissingPDF
			item.Reason = "Zotero 中没有本地 PDF"
			run.Items[i] = item
			run.Summary = summarizeZoteroItems(run.Items)
			_ = s.repo.ZoteroImport.Save(run)
			continue
		}
		paper, status, ingestErr := s.ingestZoteroItem(item, func(params UploadPaperParams) (*model.Paper, error) {
			return s.uploadPaperFromLocalPath(item.PDFPath, params)
		})
		if ingestErr != nil {
			item.Status = zoteroItemError
			item.Reason = ingestErr.Error()
		} else {
			if paper != nil {
				item.PaperID = paper.ID
			}
			item.Status = status
			item.Reason = statusReason(status)
		}
		run.Items[i] = item
		run.Summary = summarizeZoteroItems(run.Items)
		_ = s.repo.ZoteroImport.Save(run)
	}
	run.Status = zoteroRunCompleted
	_ = s.repo.ZoteroImport.Save(run)
}

func (s *LibraryService) uploadPaperFromLocalPath(path string, params UploadPaperParams) (*model.Paper, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeNotFound, "无法打开 Zotero 本地 PDF", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 Zotero PDF 失败", err)
	}
	return s.uploadPaperFromReader(file, paperUploadSource{
		Filename:     filepath.Base(path),
		ContentType:  "application/pdf",
		DeclaredSize: info.Size(),
		DOI:          params.DOI,
		TitleHint:    params.Title,
	}, params)
}

func (s *LibraryService) ingestZoteroItem(item model.ZoteroImportItem, upload func(UploadPaperParams) (*model.Paper, error)) (*model.Paper, string, error) {
	item.LibraryID = strings.TrimSpace(item.LibraryID)
	item.ItemKey = strings.TrimSpace(item.ItemKey)
	if item.ItemKey == "" {
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "缺少 Zotero item key")
	}
	if existing, err := s.repo.ExternalID.GetBySourceKey(model.ExternalSourceZotero, item.LibraryID, item.ItemKey); err != nil {
		return nil, "", err
	} else if existing != nil {
		paper, err := s.GetPaper(existing.PaperID)
		if err != nil {
			return nil, "", err
		}
		if err := s.bindZoteroExternalID(paper.ID, item); err != nil {
			return nil, "", err
		}
		return paper, zoteroItemSkippedExisting, nil
	}

	groupID, err := s.groupIDForZoteroPath(item.CollectionPath)
	if err != nil {
		return nil, "", err
	}
	paper, err := upload(UploadPaperParams{
		Title:        firstNonEmpty(item.Title, item.PDFFilename, item.ItemKey),
		DOI:          item.DOI,
		AuthorsText:  item.AuthorsText,
		Journal:      item.Journal,
		PublishedAt:  item.PublishedAt,
		AbstractText: item.AbstractText,
		GroupID:      groupID,
		Tags:         item.Tags,
	})
	status := zoteroItemImported
	if err != nil {
		var duplicateErr *DuplicatePaperError
		if !errors.As(err, &duplicateErr) || duplicateErr.Paper == nil {
			return nil, "", err
		}
		paper = duplicateErr.Paper
		status = zoteroItemSkippedExisting
	}
	if err := s.bindZoteroExternalID(paper.ID, item); err != nil {
		return nil, "", err
	}
	if status == zoteroItemImported {
		if updated, updateErr := s.applyZoteroNotes(paper, item.NotesText); updateErr == nil {
			paper = updated
		}
	}
	return paper, status, nil
}

func (s *LibraryService) applyZoteroNotes(paper *model.Paper, notes string) (*model.Paper, error) {
	notes = strings.TrimSpace(notes)
	if paper == nil || notes == "" || strings.TrimSpace(paper.PaperNotesText) != "" {
		return paper, nil
	}
	return s.UpdatePaper(paper.ID, UpdatePaperParams{
		Title:          paper.Title,
		AuthorsText:    paper.AuthorsText,
		Journal:        paper.Journal,
		PublishedAt:    paper.PublishedAt,
		AbstractText:   paper.AbstractText,
		NotesText:      paper.NotesText,
		PaperNotesText: notes,
		GroupID:        paper.GroupID,
		Tags:           paperTagNames(paper.Tags),
	})
}

func (s *LibraryService) bindZoteroExternalID(paperID int64, item model.ZoteroImportItem) error {
	return s.repo.ExternalID.Upsert(paperID, model.ExternalSourceZotero, item.LibraryID, item.ItemKey, item.CollectionPath)
}

func (s *LibraryService) groupIDForZoteroPath(path string) (*int64, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	group, err := s.GetOrCreateGroupByName(path, "Imported from Zotero collection")
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, nil
	}
	id := group.ID
	return &id, nil
}

func (s *LibraryService) planZoteroImport(ctx context.Context, collectionKeys []string, includeChildren bool) ([]model.ZoteroImportItem, error) {
	catalog, err := s.loadZoteroCatalog(ctx, true)
	if err != nil {
		return nil, err
	}
	selected := selectedCollectionSet(catalog.Collections, collectionKeys, includeChildren)
	if len(selected) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "请选择至少一个 Zotero collection")
	}
	childrenByParent := map[string][]zotero.Item{}
	for _, item := range catalog.Items {
		if item.ParentItem == "" {
			continue
		}
		childrenByParent[item.ParentItem] = append(childrenByParent[item.ParentItem], item)
	}

	client, _, err := s.zoteroClientFromSettings()
	if err != nil {
		return nil, err
	}

	out := make([]model.ZoteroImportItem, 0)
	for _, item := range catalog.Items {
		if !isRegularZoteroItem(item) {
			continue
		}
		path := collectionPathForItem(item, selected, catalog.PathByKey)
		if path == "" {
			continue
		}
		planned := model.ZoteroImportItem{
			ItemKey:        item.Key,
			LibraryID:      catalog.LibraryPrefix,
			Title:          firstNonEmpty(item.Title, item.Key),
			DOI:            item.DOI,
			AuthorsText:    formatZoteroAuthors(item.Creators),
			Journal:        firstNonEmpty(item.PublicationTitle, item.ConferenceName, item.BookTitle, item.Publisher),
			PublishedAt:    item.Date,
			AbstractText:   item.AbstractNote,
			NotesText:      joinZoteroNotes(childrenByParent[item.Key]),
			Tags:           item.Tags,
			CollectionPath: path,
			Status:         zoteroItemMissingPDF,
			Reason:         "Zotero 中没有本地 PDF",
		}
		if attachment := choosePrimaryPDF(childrenByParent[item.Key]); attachment != nil {
			planned.AttachmentKey = attachment.Key
			planned.PDFFilename = firstNonEmpty(attachment.Filename, attachment.Title)
			if pdfPath, pathErr := client.AttachmentFilePath(ctx, catalog.LibraryPrefix, attachment.Key); pathErr == nil && isLikelyPDF(pdfPath) {
				planned.HasLocalPDF = true
				planned.PDFPath = pdfPath
				planned.Status = ""
				planned.Reason = ""
			} else if pathErr != nil {
				planned.Reason = pathErr.Error()
			}
		}
		out = append(out, planned)
	}
	return out, nil
}

func (s *LibraryService) loadZoteroCatalog(ctx context.Context, includeItems bool) (*zoteroCatalog, error) {
	client, _, err := s.zoteroClientFromSettings()
	if err != nil {
		return nil, err
	}
	probed, err := client.Probe(ctx)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "无法连接 Zotero Local API", err)
	}
	collections, err := client.ListCollections(ctx, probed.LibraryPrefix)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取 Zotero collections 失败", err)
	}
	catalog := &zoteroCatalog{
		LibraryPrefix: probed.LibraryPrefix,
		Collections:   collections,
		PathByKey:     collectionPathMap(collections),
		ByKey:         map[string]zotero.Item{},
	}
	if !includeItems {
		return catalog, nil
	}
	items, err := client.ListItems(ctx, probed.LibraryPrefix)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取 Zotero 条目失败", err)
	}
	catalog.Items = items
	for _, item := range items {
		catalog.ByKey[item.Key] = item
	}
	return catalog, nil
}

func plannedItemFromInput(input model.ZoteroIngestInput) model.ZoteroImportItem {
	return model.ZoteroImportItem{
		ItemKey:        strings.TrimSpace(input.ItemKey),
		LibraryID:      firstNonEmpty(strings.TrimSpace(input.LibraryID), "users/0"),
		Title:          strings.TrimSpace(input.Title),
		DOI:            strings.TrimSpace(input.DOI),
		AuthorsText:    strings.TrimSpace(input.AuthorsText),
		Journal:        strings.TrimSpace(input.Journal),
		PublishedAt:    strings.TrimSpace(input.PublishedAt),
		AbstractText:   strings.TrimSpace(input.AbstractText),
		NotesText:      strings.TrimSpace(input.NotesText),
		Tags:           uniqueStrings(input.Tags),
		CollectionPath: strings.TrimSpace(input.CollectionPath),
	}
}

func buildZoteroCollectionTree(collections []zotero.Collection, pathByKey map[string]string) []model.ZoteroCollectionNode {
	children := map[string][]zotero.Collection{}
	var roots []zotero.Collection
	for _, collection := range collections {
		if strings.TrimSpace(collection.ParentKey) == "" {
			roots = append(roots, collection)
			continue
		}
		children[collection.ParentKey] = append(children[collection.ParentKey], collection)
	}
	var walk func(zotero.Collection) model.ZoteroCollectionNode
	walk = func(collection zotero.Collection) model.ZoteroCollectionNode {
		node := model.ZoteroCollectionNode{
			Key:       collection.Key,
			Name:      collection.Name,
			ParentKey: collection.ParentKey,
			Path:      pathByKey[collection.Key],
		}
		for _, child := range children[collection.Key] {
			node.Children = append(node.Children, walk(child))
		}
		return node
	}
	out := make([]model.ZoteroCollectionNode, 0, len(roots))
	for _, root := range roots {
		out = append(out, walk(root))
	}
	return out
}

func collectionPathMap(collections []zotero.Collection) map[string]string {
	byKey := map[string]zotero.Collection{}
	for _, collection := range collections {
		byKey[collection.Key] = collection
	}
	paths := map[string]string{}
	var pathFor func(string) string
	pathFor = func(key string) string {
		if key == "" {
			return ""
		}
		if path, ok := paths[key]; ok {
			return path
		}
		collection, ok := byKey[key]
		if !ok {
			return ""
		}
		parent := pathFor(collection.ParentKey)
		name := strings.TrimSpace(collection.Name)
		if parent == "" {
			paths[key] = name
			return name
		}
		paths[key] = parent + "/" + name
		return paths[key]
	}
	for _, collection := range collections {
		_ = pathFor(collection.Key)
	}
	return paths
}

func selectedCollectionSet(collections []zotero.Collection, keys []string, includeChildren bool) map[string]struct{} {
	selected := map[string]struct{}{}
	wanted := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			wanted[key] = struct{}{}
		}
	}
	children := map[string][]string{}
	for _, collection := range collections {
		if collection.ParentKey != "" {
			children[collection.ParentKey] = append(children[collection.ParentKey], collection.Key)
		}
	}
	var add func(string)
	add = func(key string) {
		if _, ok := selected[key]; ok {
			return
		}
		selected[key] = struct{}{}
		if !includeChildren {
			return
		}
		for _, child := range children[key] {
			add(child)
		}
	}
	for key := range wanted {
		add(key)
	}
	return selected
}

func collectionPathForItem(item zotero.Item, selected map[string]struct{}, pathByKey map[string]string) string {
	best := ""
	for _, key := range item.Collections {
		if _, ok := selected[key]; !ok {
			continue
		}
		path := pathByKey[key]
		if path == "" {
			continue
		}
		if best == "" || strings.Count(path, "/") > strings.Count(best, "/") || (strings.Count(path, "/") == strings.Count(best, "/") && path < best) {
			best = path
		}
	}
	return best
}

func isRegularZoteroItem(item zotero.Item) bool {
	switch strings.ToLower(strings.TrimSpace(item.ItemType)) {
	case "", "attachment", "note", "annotation":
		return false
	default:
		return item.ParentItem == ""
	}
}

func choosePrimaryPDF(children []zotero.Item) *zotero.Item {
	var fallback *zotero.Item
	for i := range children {
		child := children[i]
		if !isZoteroPDFAttachment(child) {
			continue
		}
		name := strings.ToLower(child.Filename + " " + child.Title)
		if strings.Contains(name, "supplement") || strings.Contains(name, "supporting") {
			if fallback == nil {
				copy := child
				fallback = &copy
			}
			continue
		}
		copy := child
		return &copy
	}
	return fallback
}

func isZoteroPDFAttachment(item zotero.Item) bool {
	if !strings.EqualFold(item.ItemType, "attachment") {
		return false
	}
	contentType := strings.ToLower(item.ContentType)
	filename := strings.ToLower(item.Filename)
	if strings.Contains(contentType, "html") || strings.Contains(filename, ".html") {
		return false
	}
	return strings.Contains(contentType, "pdf") || strings.HasSuffix(filename, ".pdf")
}

func isLikelyPDF(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return strings.HasSuffix(strings.ToLower(path), ".pdf")
	}
	defer file.Close()
	buf := make([]byte, 5)
	n, _ := io.ReadFull(file, buf)
	return n >= 4 && string(buf[:4]) == "%PDF"
}

func joinZoteroNotes(children []zotero.Item) string {
	parts := make([]string, 0, len(children))
	for _, child := range children {
		if !strings.EqualFold(child.ItemType, "note") {
			continue
		}
		note := stripHTML(child.Note)
		if note != "" {
			parts = append(parts, note)
		}
	}
	return strings.Join(parts, "\n\n")
}

func stripHTML(raw string) string {
	replaced := strings.ReplaceAll(raw, "<br>", "\n")
	replaced = strings.ReplaceAll(replaced, "<br/>", "\n")
	replaced = strings.ReplaceAll(replaced, "<br />", "\n")
	replaced = strings.ReplaceAll(replaced, "</p>", "\n")
	replaced = strings.ReplaceAll(replaced, "</div>", "\n")
	replaced = htmlTagPattern.ReplaceAllString(replaced, "")
	replaced = html.UnescapeString(replaced)
	return strings.TrimSpace(replaced)
}

func formatZoteroAuthors(creators []zotero.Creator) string {
	names := make([]string, 0, len(creators))
	for _, creator := range creators {
		if creator.CreatorType != "" && !strings.EqualFold(creator.CreatorType, "author") && !strings.EqualFold(creator.CreatorType, "editor") {
			continue
		}
		name := strings.TrimSpace(creator.Name)
		if name == "" {
			name = strings.TrimSpace(strings.TrimSpace(creator.FirstName) + " " + strings.TrimSpace(creator.LastName))
		}
		if name != "" {
			names = append(names, collapseSpaces(name))
		}
	}
	return strings.Join(names, ", ")
}

func collapseSpaces(value string) string {
	return strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
}

func summarizeZoteroItems(items []model.ZoteroImportItem) model.ZoteroImportSummary {
	summary := model.ZoteroImportSummary{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case zoteroItemImported:
			summary.Imported++
		case zoteroItemSkippedExisting:
			summary.SkippedExisting++
		case zoteroItemError:
			summary.Error++
		default:
			if !item.HasLocalPDF {
				summary.MissingPDF++
			}
		}
	}
	return summary
}

func statusReason(status string) string {
	switch status {
	case zoteroItemImported:
		return "已按 CiteBox 标准入库"
	case zoteroItemSkippedExisting:
		return "库中已存在，已绑定 Zotero 条目"
	case zoteroItemMissingPDF:
		return "Zotero 中没有本地 PDF"
	default:
		return ""
	}
}

func paperTagNames(tags []model.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		if strings.TrimSpace(tag.Name) != "" {
			names = append(names, tag.Name)
		}
	}
	return names
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func newZoteroRunID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte("zotero-fallback-id"))
	}
	return hex.EncodeToString(buf)
}
