package textractparser

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/service/textract"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
)

type OutputGenerator struct {
	s3 				*awshelper.S3Helper
	Response   		*textract.AnalyzeDocumentOutput
	DocumentId 		string
	BucketName 		string
	ObjectName 		string
	IsForms			bool
	IsTables 		bool
	OutputPath 		string
	Document 		*Document
}
func NewOutputGenerator (s3 *awshelper.S3Helper, response *textract.AnalyzeDocumentOutput, documentId, bucketName, objectName string, isForms, isTables bool) *OutputGenerator {
	outputPath := fmt.Sprintf("%s/ocr-analysis", objectName)
	document := NewDocument(response)

	return &OutputGenerator{
		s3: s3,
		DocumentId: documentId,
		Response: response,
		BucketName: bucketName,
		ObjectName: objectName,
		IsForms: isForms,
		IsTables: isTables,
		OutputPath: outputPath,
		Document: document,
	}
}

func (o *OutputGenerator) OutputText(page *Page, p int, noWrite bool) (string, error) {
	text := page.Text

	var err error
	if noWrite {
		return text, nil
	} else {
		opath := fmt.Sprintf("%s/page-%d/text.txt", o.OutputPath, p)

		err = o.s3.WriteToS3(text, o.BucketName, opath, nil)
		if err != nil {
			log.Println("Error writing detected text: ", err)
			return "", err
		}
	}

	return "", nil
}

func (o *OutputGenerator) OutputForm(page *Page, p int, noWrite bool) ([][]string, error) {
	csvData := [][]string{}
	var err error;

	for _, field := range page.Form.Fields {
		csvItem := []string{}
		if field.Key != nil {
			csvItem = append(csvItem, *field.Key.Text)
		} else {
			csvItem = append(csvItem, "")
		}
		if field.Value != nil {
			csvItem = append(csvItem, *field.Value.Text)
		} else {
			csvItem = append(csvItem, "")
		}
		csvData = append(csvData, csvItem)
	}

	if noWrite {
		return csvData, nil
	} else {
		csvFieldNames := []string{"Key", "Value"}
		opath := fmt.Sprintf("%s/page-%d/forms.csv", o.OutputPath, p)
		err = o.s3.WriteCSV(csvFieldNames, csvData, o.BucketName, opath)
		if err != nil {
			log.Println("Error writing forms: ", err)
			return nil, err
		}
	}

	return nil, nil
}

func (o *OutputGenerator) OutputTable(page *Page, p int, noWrite bool) ([][]string, error) {
	csvData := [][]string{}
	for _, table := range page.Tables {
		csvRow := []string{}
		csvRow = append(csvRow, "Table")
		csvData = append(csvData, csvRow)
		for _, row := range table.Rows {
			csvRow = []string{}
			for _, cell := range row.Cells {
				csvRow = append(csvRow, *cell.Text)
			}
			csvData = append(csvData, csvRow)
		}
		csvData = append(csvData, []string{})
		csvData = append(csvData, []string{})
	}

	if noWrite {
		return csvData, nil
	} else {
		opath := fmt.Sprintf("%s/page-%d/tables.csv", o.OutputPath, p)
		err := o.s3.WriteCSVRaw(csvData, o.BucketName, opath)
		if err != nil {
			log.Println("Error writing tables: ", err)
			return nil, err
		}
	}

	return nil, nil
}

func (o *OutputGenerator) WriteTextractOutputs(taggingStr *string) error {
	if len(o.Document.Pages) == 0 {
		return fmt.Errorf("no pages found in document %s", o.DocumentId)
	}

	p := 1
	var err error
	for _, page := range o.Document.Pages {
		// Write the raw response to S3
		opath := fmt.Sprintf("%s/page-%d/response.json", o.OutputPath, p)
		
		// Marshal the blocks into a JSON string.
		blockBytes, err := json.Marshal(page.Blocks)
		if err != nil {
			log.Println("Error serializing blocks: ", err)
			return err
		}
		blockJsonPayload := string(blockBytes)

		// Write the raw response.
		err = o.s3.WriteToS3(blockJsonPayload, o.BucketName, opath, taggingStr)
		if err != nil {
			log.Println("Error writing raw response: ", err)
			return err
		}

		// Write the formatted text to S3
		_, err = o.OutputText(page, p, false)
		if err != nil {
			return err
		}		

		// Optionally output forms.
		if o.IsForms {
			_, err = o.OutputForm(page, p, false)
			if err != nil {
				return err
			}
		}

		// Optionally output tables.
		if o.IsTables {
			_, err = o.OutputTable(page, p, false)
			if err != nil {
				return err
			}
		}

		p = p + 1
	}

	// Marshal the response into a JSON string.
	responseBytes, err := json.Marshal(o.Response)
	if err != nil {
		log.Println("Error serializing response: ", err)
		return err
	}
	responseJsonPayload := string(responseBytes)
	log.Println("Serialized Response:", responseJsonPayload)

	// Write the whole output for it to then be used for comprehend
	opath := fmt.Sprintf("%s/fullresponse.json", o.OutputPath)
	log.Println("Total Pages in Document: ", len(o.Document.Pages))
	err = o.s3.WriteToS3(responseJsonPayload, o.BucketName, opath, taggingStr)
	if(err != nil) {
		log.Println("Error writing full response: ", err)
		return err
	}

	return nil
}

