// taxonomy-gen generates Go code from taxonomy YAML files.
//
// Usage:
//
//	taxonomy-gen --base taxonomy/core.yaml --output-proto api/proto/taxonomy.proto
//
// Flags:
//
//	--base           Path to base taxonomy YAML (required)
//	--extension      Path to extension YAML (optional)
//	--output-proto   Output path for .proto file
//	--output-domain  Output path for domain Go file
//	--output-validators Output path for validators Go file
//	--output-constants  Output path for constants Go file
//	--output-helpers Output path for SDK helper functions Go file
//	--package        Go package name for generated code (default: domain)
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zero-day-ai/gibson/cmd/taxonomy-gen/generator"
	"github.com/zero-day-ai/gibson/cmd/taxonomy-gen/parser"
)

func main() {
	var (
		basePath            = flag.String("base", "", "Path to base taxonomy YAML (required)")
		extensionPath       = flag.String("extension", "", "Path to extension YAML (optional)")
		outputProto         = flag.String("output-proto", "", "Output path for .proto file")
		outputDomain        = flag.String("output-domain", "", "Output path for domain Go file")
		outputValidators    = flag.String("output-validators", "", "Output path for validators Go file")
		outputConstants     = flag.String("output-constants", "", "Output path for constants Go file")
		outputQuery         = flag.String("output-query", "", "Output path for query builders Go file")
		outputHelpers       = flag.String("output-helpers", "", "Output path for SDK helper functions Go file")
		outputRelationships = flag.String("output-relationships", "", "Output path for relationships mapping Go file")
		goPackage           = flag.String("package", "domain", "Go package name for generated code")
	)
	flag.Parse()

	if *basePath == "" {
		fmt.Fprintln(os.Stderr, "error: --base is required")
		flag.Usage()
		os.Exit(1)
	}

	// Parse base taxonomy
	fmt.Printf("Parsing base taxonomy: %s\n", *basePath)
	taxonomy, err := parser.ParseYAML(*basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing base taxonomy: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Version: %s\n", taxonomy.Version)
	fmt.Printf("  Node types: %d\n", len(taxonomy.NodeTypes))
	fmt.Printf("  Relationship types: %d\n", len(taxonomy.RelationshipTypes))
	fmt.Printf("  Techniques: %d\n", len(taxonomy.Techniques))

	// Merge extension if provided
	if *extensionPath != "" {
		fmt.Printf("Parsing extension: %s\n", *extensionPath)
		ext, err := parser.ParseYAML(*extensionPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing extension: %v\n", err)
			os.Exit(1)
		}
		taxonomy, err = parser.Merge(taxonomy, ext)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error merging extension: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  Merged node types: %d\n", len(taxonomy.NodeTypes))
		fmt.Printf("  Merged relationship types: %d\n", len(taxonomy.RelationshipTypes))
	}

	// Generate outputs
	generated := 0

	if *outputProto != "" {
		fmt.Printf("Generating proto: %s\n", *outputProto)
		if err := generator.GenerateProto(taxonomy, *outputProto); err != nil {
			fmt.Fprintf(os.Stderr, "error generating proto: %v\n", err)
			os.Exit(1)
		}
		generated++
	}

	if *outputDomain != "" {
		fmt.Printf("Generating domain types: %s\n", *outputDomain)
		if err := generator.GenerateDomain(taxonomy, *outputDomain, *goPackage); err != nil {
			fmt.Fprintf(os.Stderr, "error generating domain: %v\n", err)
			os.Exit(1)
		}
		generated++
	}

	if *outputValidators != "" {
		fmt.Printf("Generating validators: %s\n", *outputValidators)
		if err := generator.GenerateValidators(taxonomy, *outputValidators, *goPackage); err != nil {
			fmt.Fprintf(os.Stderr, "error generating validators: %v\n", err)
			os.Exit(1)
		}
		generated++
	}

	if *outputConstants != "" {
		fmt.Printf("Generating constants: %s\n", *outputConstants)
		if err := generator.GenerateConstants(taxonomy, *outputConstants, *goPackage); err != nil {
			fmt.Fprintf(os.Stderr, "error generating constants: %v\n", err)
			os.Exit(1)
		}
		generated++
	}

	if *outputQuery != "" {
		fmt.Printf("Generating query builders: %s\n", *outputQuery)
		if err := generator.GenerateQueryBuilders(taxonomy, *outputQuery, *goPackage); err != nil {
			fmt.Fprintf(os.Stderr, "error generating query builders: %v\n", err)
			os.Exit(1)
		}
		generated++
	}

	if *outputHelpers != "" {
		fmt.Printf("Generating SDK helpers: %s\n", *outputHelpers)
		if err := generator.GenerateHelpers(taxonomy, *outputHelpers, *goPackage); err != nil {
			fmt.Fprintf(os.Stderr, "error generating helpers: %v\n", err)
			os.Exit(1)
		}
		generated++
	}

	if *outputRelationships != "" {
		fmt.Printf("Generating relationships mapping: %s\n", *outputRelationships)
		if err := generator.GenerateRelationships(taxonomy, *outputRelationships, *goPackage); err != nil {
			fmt.Fprintf(os.Stderr, "error generating relationships: %v\n", err)
			os.Exit(1)
		}
		generated++
	}

	if generated == 0 {
		fmt.Println("Warning: no output files specified. Use --output-* flags to generate files.")
	} else {
		fmt.Printf("\nGeneration complete! Generated %d file(s).\n", generated)
	}
}
