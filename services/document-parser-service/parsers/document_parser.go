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
	p.supportedFormats[".doc"] = p.parseDOC
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

	var allText strings.Builder

	// Список XML файлов, из которых извлекаем текст (document.xml + headers/footers)
	xmlTargets := []string{
		"word/document.xml",
		"word/header1.xml", "word/header2.xml", "word/header3.xml",
		"word/footer1.xml", "word/footer2.xml", "word/footer3.xml",
	}

	for _, target := range xmlTargets {
		for _, file := range zipReader.File {
			if file.Name == target {
				xmlFile, err := file.Open()
				if err != nil {
					continue
				}
				xmlData, err := io.ReadAll(xmlFile)
				xmlFile.Close()
				if err != nil {
					continue
				}
				text, err := extractTextFromDocumentXML(xmlData)
				if err != nil {
					continue
				}
				if text != "" {
					allText.WriteString(text)
					allText.WriteString("\n\n")
				}
			}
		}
	}

	result := strings.TrimSpace(allText.String())
	if result == "" {
		return "", fmt.Errorf("не удалось извлечь текст из DOCX файла")
	}
	return result, nil
}

// extractTextFromDocumentXML рекурсивно извлекает весь текст из OOXML,
// включая параграфы внутри таблиц, текстовых блоков и вложенных структур.
func extractTextFromDocumentXML(xmlData []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	var text strings.Builder
	var inParagraph bool
	var paragraphText strings.Builder

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return text.String(), nil // return what we have
		}

		switch t := token.(type) {
		case xml.StartElement:
			localName := t.Name.Local
			switch localName {
			case "p": // <w:p> — paragraph
				inParagraph = true
				paragraphText.Reset()
			case "tab": // <w:tab> — tab character
				if inParagraph {
					paragraphText.WriteString("\t")
				}
			case "br": // <w:br> — line break
				if inParagraph {
					paragraphText.WriteString("\n")
				}
			}
		case xml.EndElement:
			localName := t.Name.Local
			if localName == "p" && inParagraph {
				line := paragraphText.String()
				text.WriteString(line)
				text.WriteString("\n")
				inParagraph = false
			}
		case xml.CharData:
			if inParagraph {
				paragraphText.Write(t)
			}
		}
	}

	return strings.TrimSpace(text.String()), nil
}

// parseDOC handles legacy .doc files.
// Many modern tools save .doc as DOCX (Office Open XML) internally,
// so we try DOCX parsing first. If that fails, we extract raw text from the binary.
func (p *DocumentParser) parseDOC(content []byte) (string, error) {
	// Try DOCX first — some .doc files are actually DOCX with wrong extension
	text, err := p.parseDOCX(content)
	if err == nil && len(strings.TrimSpace(text)) > 0 {
		return text, nil
	}

	// Fallback: extract readable text from binary .doc
	// Legacy .doc is a binary OLE2 format; extract ASCII/UTF-8 text runs
	var result strings.Builder
	var current strings.Builder
	for _, b := range content {
		if b >= 32 && b < 127 || b == '\n' || b == '\r' || b == '\t' {
			current.WriteByte(b)
		} else if b >= 0xC0 {
			// Possible UTF-8 multibyte start — include it
			current.WriteByte(b)
		} else if b >= 0x80 && b <= 0xBF {
			// UTF-8 continuation byte
			current.WriteByte(b)
		} else {
			// Non-text byte: flush current run if long enough
			run := strings.TrimSpace(current.String())
			if len(run) >= 10 {
				result.WriteString(run)
				result.WriteString("\n")
			}
			current.Reset()
		}
	}
	// Flush remaining
	run := strings.TrimSpace(current.String())
	if len(run) >= 10 {
		result.WriteString(run)
		result.WriteString("\n")
	}

	extracted := strings.TrimSpace(result.String())
	if len(extracted) < 20 {
		return "", fmt.Errorf("не удалось извлечь текст из .doc файла. Рекомендуется сконвертировать в .docx")
	}
	return extracted, nil
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
