package osbuild2

import "fmt"

type PipelineType string

const (
	QEMUPipeline PipelineType = "org.osbuild.pipeline.qemu"
)

type PipelineTypeConfig interface {
}

type PipelineGenerator interface {
	Type() PipelineType
	Pipeline(name, build, runner string) (*Pipeline, error)
}

var pipelineGeneratorsRegistry map[PipelineType]PipelineGenerator = map[PipelineType]PipelineGenerator{
	QEMUPipeline: qemuPipelineGenerator,
}

func GetPipeline(name, build, runner string, pipelineType PipelineType, config PipelineTypeConfig) (*Pipeline, error) {
	generator, ok := pipelineGeneratorsRegistry[pipelineType]
	if !ok {
		return nil, fmt.Errorf("unknown pipeline type: %q", pipelineType)
	}

	pipeline, err := generator(name, build, runner, config)
	if err != nil {
		return nil, err
	}

	return pipeline, nil
}

// *** QEMU PIPELINE ***

type QEMUPipelineConfig struct {
	Format QEMUFormat
}

func (c *QEMUPipelineConfig) Pipeline(name, build, runner string) (*Pipeline, error) {
	p := NewPipeline(name, build, runner)
	p.Name = string(format)
	p.Build = "name:build"

	qemuStage := osbuild.NewQEMUStage(
		osbuild.NewQEMUStageOptions(outputFilename, format, formatOptions),
		osbuild.NewQemuStagePipelineFilesInputs(inputPipelineName, inputFilename),
	)
	p.AddStage(qemuStage)
	return p
	return nil, nil
}
