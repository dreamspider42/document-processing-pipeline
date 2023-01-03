package textractparser

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/textract"
)

// An axis-aligned coarse representation of the location of the recognized item on the document page.
type BoundingBox struct {
	Width  *float64 `json:"Width"`
	Height *float64 `json:"Height"`
	Left   *float64 `json:"Left"`
	Top    *float64 `json:"Top"`
}
func(b *BoundingBox) String() string {
	return fmt.Sprintf("width: %f, height: %f, left: %f, top: %f", *b.Width, *b.Height, *b.Left, *b.Top)
}

// Within the bounding box, a fine-grained polygon around the recognized item.
type Polygon struct {
	X *float64 `json:"X"`
	Y *float64 `json:"Y"`
}
func(p *Polygon) String() string {
	return fmt.Sprintf("x: %f, y: %f", *p.X, *p.Y)
}

// The location of the recognized text on the image. 
// It includes an axis-aligned, coarse bounding box that surrounds the text, and a finer-grain polygon for more accurate spatial information.
type Geometry struct {
	BoundingBox *BoundingBox `json:"BoundingBox"`
	Polygon     []Polygon    `json:"Polygon"`
}
func NewGeometry(geo *textract.Geometry) *Geometry {
	bb := BoundingBox{geo.BoundingBox.Width, geo.BoundingBox.Height, geo.BoundingBox.Left, geo.BoundingBox.Top}
	pgs := make([]Polygon, 0)
	for _, pg := range geo.Polygon {
		pgs = append(pgs, Polygon{pg.X, pg.Y})
	}

	return &Geometry{
		BoundingBox: &bb,
		Polygon:     pgs,
	}
}
func(g *Geometry) String() string {
	s := fmt.Sprintf("BoundingBox: %s\n", g.BoundingBox)
	return s
}

// A word that's detected on a document page. 
// A word is one or more ISO basic Latin script characters that aren't separated by spaces.
type Word struct {
	Block      *textract.Block `json:"Block"`
	Confidence *float64        `json:"Confidence"`
	Geometry   *Geometry       `json:"Geometry"`
	Id         *string         `json:"Id"`
	Text       *string         `json:"Text"`
}
func NewWord(block *textract.Block) *Word {
	return &Word{
		Block:      block,
		Confidence: block.Confidence,
		Geometry:   NewGeometry(block.Geometry),
		Id:         block.Id,
		Text:       block.Text,
	}
}
func(w *Word) String() string {
	if(w.Text == nil) {
		return ""
	}
	return fmt.Sprintf(*w.Text)
}

// A string of tab-delimited, contiguous words that are detected on a document page.
type Line struct {
	Block      *textract.Block `json:"Block"`
	Confidence *float64        `json:"Confidence"`
	Geometry   *Geometry       `json:"Geometry"`
	Id         *string         `json:"Id"`
	Text       *string         `json:"Text"`
	Words      []*Word         `json:"Words"`
}
func NewLine(block *textract.Block, blockMap map[string]*textract.Block) *Line {
	words := make([]*Word, 0)
	if(block.Relationships != nil) {
		for _, rs := range block.Relationships {
			if(*rs.Type == "CHILD") {
				for _, cid := range rs.Ids {
					if _, ok := blockMap[*cid]; ok {
						if(*blockMap[*cid].BlockType == "WORD") {
							words = append(words, NewWord(blockMap[*cid]))
						}
					}
				}
			}
		}
	}

	return &Line{
		Block:      block,
		Confidence: block.Confidence,
		Geometry:   NewGeometry(block.Geometry),
		Id:         block.Id,
		Text:       block.Text,
		Words:      words,
	}
}
func(l *Line) String() string {

	s := "Line\n==========\n"
	s = s + *l.Text + "\n"
	s = s + "Words\n----------\n"
	for _, word := range l.Words {
		s = s + fmt.Sprintf("[%s]", word)
	}
	return s
}

// A selection element such as an option button (radio button) or a check box that's detected on a document page.
// Use the value of SelectionStatus to determine the status of the selection element.
type SelectionElement struct {
	Block           *textract.Block `json:"Block"`
	Confidence      *float64        `json:"Confidence"`
	Geometry        *Geometry       `json:"Geometry"`
	Id              *string         `json:"Id"`
	SelectionStatus *string         `json:"SelectionStatus"`
}
func NewSelectionElement(block *textract.Block) *SelectionElement {
	return &SelectionElement{
		Block:           block,
		Confidence:      block.Confidence,
		Geometry:        NewGeometry(block.Geometry),
		Id:              block.Id,
		SelectionStatus: block.SelectionStatus,
	}
}

// Stores the KEY Block objects for linked text that's detected on a document page.
type FieldKey struct {
	Block    *textract.Block `json:"Block"`
	Content  []*Word         `json:"Content"`
	Confidence *float64       `json:"Confidence"`
	Geometry   *Geometry      `json:"Geometry"`
	Id         *string        `json:"Id"`
	Text       *string        `json:"Text"`
}
func NewFieldKey(block *textract.Block, children []*string, blockMap map[string]*textract.Block) *FieldKey {
	content := make([]*Word, 0)
	t := make([]string, 0)
	for _, eid := range children {
		if _, ok := blockMap[*eid]; ok {
			if(*blockMap[*eid].BlockType == "WORD") {
				w := NewWord(blockMap[*eid])
				content = append(content, w)
				t = append(t, *w.Text)
			}
		}
	}

	return &FieldKey{
		Block:    block,
		Content:  content,
		Confidence: block.Confidence,
		Geometry:   NewGeometry(block.Geometry),
		Id:         block.Id,
		Text:       aws.String(strings.Join(t, " ")),
	}
}
func(fk *FieldKey) String() string {
	if(fk.Text == nil) {
		return ""
	}
	return fmt.Sprintf(*fk.Text)
}

// Stores the VALUE Block objects for linked text that's detected on a document page.
type FieldValue struct {
	Block    *textract.Block `json:"Block"`
	Content  []interface{}   `json:"Content"`
	Confidence *float64       `json:"Confidence"`
	Geometry   *Geometry      `json:"Geometry"`
	Id         *string        `json:"Id"`
	Text       *string        `json:"Text"`
}
func NewFieldValue(block *textract.Block, children []*string, blockMap map[string]*textract.Block) *FieldValue {
	content := make([]interface{}, 0)
	t := make([]string, 0)
	for _, eid := range children {
		if _, ok := blockMap[*eid]; ok {
			if(*blockMap[*eid].BlockType == "WORD") {
				w := NewWord(blockMap[*eid])
				content = append(content, w)
				t = append(t, *w.Text)
			} else if(*blockMap[*eid].BlockType == "SELECTION_ELEMENT") {
				se := NewSelectionElement(blockMap[*eid])
				content = append(content, se)
				t = append(t, *se.SelectionStatus)
			}
		}
	}

	return &FieldValue{
		Block:    block,
		Content:  content,
		Confidence: block.Confidence,
		Geometry:   NewGeometry(block.Geometry),
		Id:         block.Id,
		Text:       aws.String(strings.Join(t, " ")),
	}
}
func(fv *FieldValue) String() string {
	if(fv.Text == nil) {
		return ""
	}
	return fmt.Sprintf(*fv.Text)
}

// KEY_VALUE_SET - Stores the KEY and VALUE Block objects for linked text that's detected on a document page. 
// Use the EntityType field to determine if a KEY_VALUE_SET object is a KEY Block object or a VALUE Block object.
type Field struct {
	Key   *FieldKey   `json:"Key"`
	Value *FieldValue `json:"Value"`
}
func NewField(block *textract.Block, blockMap map[string]*textract.Block) *Field {
	var key *FieldKey
	var value *FieldValue

	for _, item := range block.Relationships {
		if(*item.Type == "CHILD") {
			key = NewFieldKey(block, item.Ids, blockMap)
		} else if(*item.Type == "VALUE") {
			for _, eid := range item.Ids {
				if _, ok := blockMap[*eid]; ok {
					vkvs := blockMap[*eid]
					if refsContainValue(vkvs.EntityTypes, "VALUE") {
						if(vkvs.Relationships != nil) {
							for _, vitem := range vkvs.Relationships {
								if(*vitem.Type == "CHILD") {
									value = NewFieldValue(vkvs, vitem.Ids, blockMap)
								}
							}
						}
					}
				}
			}
		}
	}

	return &Field{
		Key:   key,
		Value: value,
	}
}
func (f *Field) String() string {
	s := "\nField\n==========\n"
	k := ""
	v := ""
	if(f.Key != nil) {
		k = f.Key.String()
	}
	if(f.Value != nil) {
		v = f.Value.String()
	}
	s = s + "Key: " + k + "\nValue: " + v
	return s
}

// Collection of fields detected on a document page.
type Form struct {
	Fields    []*Field `json:"Fields"`
	FieldsMap map[string]*Field
}
func NewForm() *Form {
	return &Form{
		Fields:    make([]*Field, 0),
		FieldsMap: make(map[string]*Field),
	}
}
func (f *Form) AddField(field *Field) {
	f.Fields = append(f.Fields, field)
	f.FieldsMap[*field.Key.Text] = field
}
func (f *Form) GetFieldByKey(key string) *Field {
	field := &Field{}
	if _, ok := f.FieldsMap[key]; ok {
		field = f.FieldsMap[key]
	}
	return field
}
func (f *Form) SearchFieldsByKey(key string) []*Field {
	searchKey := strings.ToLower(key)
	results := make([]*Field, 0)
	for _, field := range f.Fields {
		if(field.Key != nil && strings.Contains(strings.ToLower(*field.Key.Text), searchKey)) {
			results = append(results, field)
		}
	}
	return results
}
func (f *Form) String() string {
	s := ""
	for _, field := range f.Fields {
		s = s + field.String() + "\n"
	}
	return s
}

// A cell within a detected table. The cell is the parent of the block that contains the text in the cell.
type Cell struct {
	Block          *textract.Block
	Confidence     *float64
	RowIndex       *int64
	ColumnIndex    *int64
	RowSpan        *int64
	ColumnSpan     *int64
	Geometry       *Geometry
	Id             *string
	Content        []interface{}
	Text           *string
}
func NewCell(block *textract.Block, blockMap map[string]*textract.Block) *Cell {
	var content []interface{}
	var text string

	for _, item := range block.Relationships {
		if(*item.Type == "CHILD") {
			for _, cid := range item.Ids {
				if _, ok := blockMap[*cid]; ok {
					switch *blockMap[*cid].BlockType {
					case "WORD":
						w := NewWord(blockMap[*cid])
						content = append(content, w)
						text = text + *w.Text + " "
					case "SELECTION_ELEMENT":
						se := NewSelectionElement(blockMap[*cid])
						content = append(content, se)
						text = text + *se.SelectionStatus + ", "
					}
				}
			}
		}
	}

	return &Cell{
		Block:       block,
		Confidence:  block.Confidence,
		RowIndex:    block.RowIndex,
		ColumnIndex: block.ColumnIndex,
		RowSpan:     block.RowSpan,
		ColumnSpan:  block.ColumnSpan,
		Geometry:    NewGeometry(block.Geometry),
		Id:          block.Id,
		Content:     content,
		Text:        aws.String(text),
	}
}
func (c *Cell) String() string {
	if c.Text == nil {
		return ""
	}
	return fmt.Sprintf(*c.Text)
}

// A horizonal row of cells within a detected table.
type Row struct {
	Cells []*Cell
}
func NewRow() *Row {
	return &Row{
		Cells: make([]*Cell, 0),
	}
}
func (r *Row) String() string {
	s := ""
	for _, cell := range r.Cells {
		s = s + fmt.Sprintf("[%s]", cell)
	}
	return s
}

// A table that's detected on a document page. 
// A table is grid-based information with two or more rows or columns, with a cell span of one row and one column each.
type Table struct{
	Block *textract.Block
	Confidence *float64
	Geometry *Geometry
	Id *string
	Rows []*Row
}
func NewTable(block *textract.Block, blockMap map[string]*textract.Block) *Table {
	table := &Table{
		Block: block,
		Confidence: block.Confidence,
		Geometry: NewGeometry(block.Geometry),
		Id: block.Id,
		Rows: make([]*Row, 0),
	}

	// Initialize the rows
	ri := int64(1)
	row := NewRow()

	// Work through blocks and add cells to rows
	var cell *Cell
	if(block.Relationships != nil) {
		for _, rs := range block.Relationships {
			if(*rs.Type == "CHILD") {
				for _, cid := range rs.Ids {
					if _, ok := blockMap[*cid]; ok {
						cell = NewCell(blockMap[*cid], blockMap)
						if(*cell.RowIndex > ri) {
							table.Rows = append(table.Rows, row)
							row = NewRow()
							ri = *cell.RowIndex
						}
						row.Cells = append(row.Cells, cell)
					}
				}
				if(row != nil && row.Cells != nil) {
					table.Rows = append(table.Rows, row)
				}
			}
		}
	}

	return table
}
func (t *Table) String() string {
	s := "Table\n==========\n"
	for _, row := range t.Rows {
		s = s + "Row\n==========\n"
		s = s + row.String() + "\n"
	}
	return s
}

// Contains a list of child Block objects that are detected on a document page.
type Page struct {
	Blocks []*textract.Block
	Text string
	Lines []*Line
	Form *Form
	Tables []*Table
	Content []interface{}
	Geometry *Geometry
	Id *string
}
func NewPage(blocks []*textract.Block, blockMap map[string]*textract.Block) *Page {
	page := &Page{
		Blocks: blocks,
		Text: "",
		Lines: make([]*Line, 0),
		Form: NewForm(),
		Tables: make([]*Table, 0),
		Content: make([]interface{}, 0),
	}

	// Parse the blocks
	page.parse(blockMap)

	return page
}
// Parse the blocks and populate the page
func (p *Page) parse(blockMap map[string]*textract.Block) {
	for _, item := range p.Blocks {
		if(*item.BlockType == "PAGE") {
			p.Geometry = NewGeometry(item.Geometry)
			p.Id = item.Id
		} else if(*item.BlockType == "LINE") {
			l := NewLine(item, blockMap)
			p.Lines = append(p.Lines, l)
			p.Content = append(p.Content, l)
			p.Text = p.Text + *l.Text + "\n"
		} else if(*item.BlockType == "TABLE") {
			t := NewTable(item, blockMap)
			p.Tables = append(p.Tables, t)
			p.Content = append(p.Content, t)
		} else if(*item.BlockType == "KEY_VALUE_SET") {
			if refsContainValue(item.EntityTypes, "KEY") {
				f := NewField(item, blockMap)
				if(f.Key != nil) {
					p.Form.AddField(f)
					p.Content = append(p.Content, f)
				} else {
					fmt.Println("WARNING: Detected K/V where key does not have content. Excluding key from output.")
					fmt.Println(f)
					fmt.Println(item)
				}
			}
		}
	}
}
func (p *Page) String() string {
	s := "Page\n==========\n"
	for _, item := range p.Content {
		s = s + fmt.Sprint(item) + "\n"
	}

	return s
}


// Represents a textracted document broken into logical sections
type Document struct {
	ResponsePages 			[]*textract.AnalyzeDocumentOutput
	Pages         			[]*Page
	ResponseDocumentPages 	[][]*textract.Block
	BlockMap 				map[string]*textract.Block
}
func NewDocument(response *textract.AnalyzeDocumentOutput) *Document {
	responsePages := make([]*textract.AnalyzeDocumentOutput, 0)
	responsePages = append(responsePages, response)

	d := &Document{
		ResponsePages: responsePages,
		Pages:         make([]*Page, 0),

	}

	// Properly breakdown the blocks and organize them by page.
	d.ResponseDocumentPages, d.BlockMap = d.parseDocumentPagesAndBlockMap()
	for _, documentPage := range d.ResponseDocumentPages {
		page := NewPage(documentPage, d.BlockMap)
		d.Pages = append(d.Pages, page)
	}

	return d
}
// Parse the document pages and block map
func (d *Document) parseDocumentPagesAndBlockMap() ([][]*textract.Block, map[string]*textract.Block) {
	blockMap := make(map[string]*textract.Block)
	documentPages := make([][]*textract.Block, 0)
	documentPage := make([]*textract.Block, 0)

	for _, page := range d.ResponsePages {
		for _, block := range page.Blocks {
			if(block.BlockType != nil && block.Id != nil) {
				blockMap[*block.Id] = block
			}

			if(*block.BlockType == "PAGE") {
				if(len(documentPage) > 0) {
					documentPages = append(documentPages, documentPage)
				}
				documentPage = make([]*textract.Block, 0)
				documentPage = append(documentPage, block)
			} else {
				documentPage = append(documentPage, block)
			}
		}
	}
	if(len(documentPage) > 0) {
		if(len(documentPage) > 0) {
			documentPages = append(documentPages, documentPage)
		}
	}

	return documentPages, blockMap
}

// Utility function to check if a slice of string pointers contains a value
func refsContainValue(in []*string, val string) bool {
	for _, v := range in {
		if *v == val {
			return true
		}
	}
	return false
}