.PHONY: build clean deploy

build:
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/documentIngest src/lambda/document_ingest/document_ingest.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/documentRegister src/lambda/document_register/document_register.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/documentLineage src/lambda/document_lineage/document_lineage.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/documentClassifier src/lambda/document_classifier/document_classifier.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/documentTracking src/lambda/document_tracking/document_tracking.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/documentProcessor src/lambda/document_processor/document_processor.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/textractSync src/lambda/textract_sync/textract_sync.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/textractAsyncStarter src/lambda/textract_async_starter/textract_async_starter.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/textractAsyncProcessor src/lambda/textract_async_processor/textract_async_processor.go
	env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/comprehendProcessor src/lambda/comprehend_processor/comprehend_processor.go
clean:
	rm -rf ./bin ./vendor Gopkg.lock
deploy: clean build
	sls deploy --verbose --aws-profile profilename
format: 
	gofmt -w src/lambda/document_ingest/document_ingest.go
	gofmt -w src/lambda/document_register/document_register.go
	gofmt -w src/lambda/document_lineage/document_lineage.go
	gofmt -w src/lambda/document_classifier/document_classifier.go
	gofmt -w src/lambda/document_tracking/document_tracking.go
	gofmt -w src/lambda/document_processor/document_processor.go
	gofmt -w src/lambda/textract_sync/textract_sync.go
	gofmt -w src/lambda/textract_async_starter/textract_async_starter.go
	gofmt -w src/lambda/textract_async_processor/textract_async_processor.go
	gofmt -w src/lambda/comprehend_processor/comprehend_processor.go
remove:
	sls remove --verbose --aws-profile profilename
