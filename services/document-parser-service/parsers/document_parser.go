package parsers

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

type DocumentParser struct {
	supportedFormats map[string]ParserFunc
}

type ParserFunc func(content []byte) (string, error)

func NewDocumentParser() *DocumentParser {
	p := &DocumentParser{
		supportedFormats: make(map[string]ParserFunc),
	}
	p.supportedFormats[".txt"] = p.parseTXT
	p.supportedFormats[".pdf"] = p.parsePDF
	p.supportedFormats[".docx"] = p.parseDOCX
	p.supportedFormats[".json"] = p.parseJSON
	p.supportedFormats[".csv"] = p.parseCSV
	p.supportedFormats[".xlsx"] = p.parseXLSX
	p.supportedFormats[".xls"] = p.parseXLSX
	p.supportedFormats[".html"] = p.parseHTML
	p.supportedFormats[".htm"] = p.parseHTML
	p.supportedFormats[".md"] = p.parseMarkdown
	return p
}

func (p *DocumentParser) ParseFile(content []byte, filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	parserFunc, ok := p.supportedFormats[ext]
	if !ok {
		return "", fmt.Errorf("формат %s не поддерживается", ext)
	}
	text, err := parserFunc(content)
	if err != nil {
		return "", fmt.Errorf("ошибка при парсинге файла %s: %w", filename, err)
	}
	return text, nil
}

func (p *DocumentParser) getSupportedFormats() []string {
	formats := make([]string, 0, len(p.supportedFormats))
	for format := range p.supportedFormats {
		formats = append(formats, format)
	}
	return formats
}

func (p *DocumentParser) parseTXT(content []byte) (string, error) {
	return string(content), nil
}

func (p *DocumentParser) parsePDF(content []byte) (string, error) {
	reader := bytes.NewReader(content)
	pdfReader, err := pdf.NewReader(reader, int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("не удалось открыть PDF: %w", err)
	}
	var text strings.Builder
	numPages := pdfReader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text.WriteString(pageText)
		text.WriteString("\n\n")
	}
	return strings.TrimSpace(text.String()), nil
}

func (p *DocumentParser) parseDOCX(content []byte) (string, error) {
	// DOCX это ZIP архив с XML файлами
	reader := bytes.NewReader(content)
	zipReader, err := zip.NewReader(reader, int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("не удалось открыть DOCX как ZIP: %w", err)
	}

	// Ищем word/document.xml
	var documentXML *zip.File
	for _, file := range zipReader.File {
		if file.Name == "word/document.xml" {
			documentXML = file
			break
		}
	}

	if documentXML == nil {
		return "", fmt.Errorf("не найден word/document.xml в DOCX файле")
	}

	// Читаем XML
	xmlFile, err := documentXML.Open()
	if err != nil {
		return "", fmt.Errorf("не удалось открыть document.xml: %w", err)
	}
	defer xmlFile.Close()

	xmlData, err := io.ReadAll(xmlFile)
	if err != nil {
		return "", fmt.Errorf("не удалось прочитать document.xml: %w", err)
	}

	// Парсим XML и извлекаем текст
	return extractTextFromDocumentXML(xmlData)
}

// extractTextFromDocumentXML извлекает текст из word/document.xml
func extractTextFromDocumentXML(xmlData []byte) (string, error) {
	type Text struct {
		Value string `xml:",chardata"`
	}
	type Run struct {
		Text []Text `xml:"t"`
	}
	type Paragraph struct {
		Runs []Run `xml:"r"`
	}
	type Body struct {
		Paragraphs []Paragraph `xml:"p"`
	}
	type Document struct {
		Body Body `xml:"body"`
	}

	var doc Document
	if err := xml.Unmarshal(xmlData, &doc); err != nil {
		return "", fmt.Errorf("не удалось распарсить XML: %w", err)
	}

	var text strings.Builder
	for _, para := range doc.Body.Paragraphs {
		for _, run := range para.Runs {
			for _, t := range run.Text {
				text.WriteString(t.Value)
			}
		}
		text.WriteString("\n")
	}

	return strings.TrimSpace(text.String()), nil
}

func (p *DocumentParser) parseJSON(content []byte) (string, error) {
	var data interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return "", fmt.Errorf("невалидный JSON: %w", err)
	}
	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

func (p *DocumentParser) parseCSV(content []byte) (string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("не удалось прочитать CSV: %w", err)
	}
	var text strings.Builder
	for _, row := range records {
		text.WriteString(strings.Join(row, ", "))
		text.WriteString("\n")
	}
	return strings.TrimSpace(text.String()), nil
}

func (p *DocumentParser) parseXLSX(content []byte) (string, error) {
	reader := bytes.NewReader(content)
	f, err := excelize.OpenReader(reader)
	if err != nil {
		return "", fmt.Errorf("не удалось открыть Excel: %w", err)
	}
	defer f.Close()
	var text strings.Builder
	sheets := f.GetSheetList()
	for _, sheet := range sheets {
		text.WriteString(fmt.Sprintf("=== Лист: %s ===\n", sheet))
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			text.WriteString(strings.Join(row, ", "))
			text.WriteString("\n")
		}
		text.WriteString("\n")
	}
	return strings.TrimSpace(text.String()), nil
}

func (p *DocumentParser) parseHTML(content []byte) (string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("не удалось распарсить HTML: %w", err)
	}
	doc.Find("script, style").Remove()
	text := doc.Find("body").Text()
	if text == "" {
		text = doc.Text()
	}
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}
	return strings.Join(cleanedLines, "\n"), nil
}

func (p *DocumentParser) parseMarkdown(content []byte) (string, error) {
	// Для markdown просто возвращаем исходный текст
	// так как он уже читаемый и не содержит HTML разметки
	return string(content), nil
}
