package handlers

import (
	"io"
	"mime/multipart"

	"document-parser-service/parsers"
	"github.com/gofiber/fiber/v2"
)

type DocumentHandler struct {
	parser *parsers.DocumentParser
}

func NewDocumentHandler() *DocumentHandler {
	return &DocumentHandler{
		parser: parsers.NewDocumentParser(),
	}
}

type ParseResponse struct {
	Text     string `json:"text"`
	FileName string `json:"file_name"`
	FileType string `json:"file_type"`
	Size     int64  `json:"size"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *DocumentHandler) ParseDocument(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "Файл не найден в запросе",
		})
	}

	src, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "Не удалось открыть файл: " + err.Error(),
		})
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "Не удалось прочитать файл: " + err.Error(),
		})
	}

	text, err := h.parser.ParseFile(content, file.Filename)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: err.Error(),
		})
	}

	return c.JSON(ParseResponse{
		Text:     text,
		FileName: file.Filename,
		FileType: getFileType(file),
		Size:     file.Size,
	})
}

func getFileType(file *multipart.FileHeader) string {
	contentType := file.Header.Get("Content-Type")
	if contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}
