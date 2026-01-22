package graphrag

import (
	"github.com/zero-day-ai/sdk/graphrag"
)

// simpleTaxonomy is a basic implementation of graphrag.TaxonomyReader.
type simpleTaxonomy struct {
	version       string
	nodeTypes     map[string]bool
	relationTypes map[string]bool
}

// NewSimpleTaxonomy creates a TaxonomyReader using SDK constants.
func NewSimpleTaxonomy() graphrag.TaxonomyReader {
	return &simpleTaxonomy{
		version:       graphrag.TaxonomyVersion,
		nodeTypes:     buildNodeTypes(),
		relationTypes: buildRelationTypes(),
	}
}

func (t *simpleTaxonomy) Version() string {
	return t.version
}

func (t *simpleTaxonomy) IsCanonicalNodeType(typeName string) bool {
	return t.nodeTypes[typeName]
}

func (t *simpleTaxonomy) IsCanonicalRelationType(typeName string) bool {
	return t.relationTypes[typeName]
}

func (t *simpleTaxonomy) ValidateNodeType(typeName string) bool {
	return true
}

func (t *simpleTaxonomy) ValidateRelationType(typeName string) bool {
	return true
}

func buildNodeTypes() map[string]bool {
	return map[string]bool{
		graphrag.NodeTypeAgentRun:      true,
		graphrag.NodeTypeMission:       true,
		graphrag.NodeTypeMissionRun:    true,
		graphrag.NodeTypeToolExecution: true,
		graphrag.NodeTypeLlmCall:       true,
		graphrag.NodeTypeDomain:        true,
		graphrag.NodeTypeSubdomain:     true,
		graphrag.NodeTypeHost:          true,
		graphrag.NodeTypePort:          true,
		graphrag.NodeTypeService:       true,
		graphrag.NodeTypeCertificate:   true,
		graphrag.NodeTypeTechnology:    true,
		graphrag.NodeTypeEndpoint:      true,
		graphrag.NodeTypeApi:           true,
		graphrag.NodeTypeFinding:       true,
		graphrag.NodeTypeEvidence:      true,
		graphrag.NodeTypeTechnique:     true,
		graphrag.NodeTypeTactic:        true,
		graphrag.NodeTypeMitigation:    true,
	}
}

func buildRelationTypes() map[string]bool {
	return map[string]bool{
		graphrag.RelTypePartOf:            true,
		graphrag.RelTypeExecutedBy:        true,
		graphrag.RelTypeDiscovered:        true,
		graphrag.RelTypeProduced:          true,
		graphrag.RelTypeHasSubdomain:      true,
		graphrag.RelTypeResolvesTo:        true,
		graphrag.RelTypeHasPort:           true,
		graphrag.RelTypeRunsService:       true,
		graphrag.RelTypeHasEndpoint:       true,
		graphrag.RelTypeUsesTechnology:    true,
		graphrag.RelTypeServesCertificate: true,
		graphrag.RelTypeAffects:           true,
		graphrag.RelTypeExploits:          true,
		graphrag.RelTypeUsesTechnique:     true,
		graphrag.RelTypeHasEvidence:       true,
		graphrag.RelTypeMitigates:         true,
	}
}
