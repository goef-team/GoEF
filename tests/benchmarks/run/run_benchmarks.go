package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run run_benchmarks.go <benchmark_type>")
		fmt.Println("Available benchmark types: all, context, change, query, metadata, dialect, load")
		os.Exit(1)
	}

	benchmarkType := os.Args[1]

	benchmarks := map[string][]string{
		"context": {
			"BenchmarkDbContext_Add",
			"BenchmarkDbContext_SaveChanges_Single",
			"BenchmarkDbContext_SaveChanges_Batch",
			"BenchmarkDbContext_Update",
			"BenchmarkDbContext_Find",
			"BenchmarkDbContext_Transaction",
			"BenchmarkDbContext_ConcurrentAdd",
			"BenchmarkDbContext_MemoryAllocation",
			"BenchmarkDbContext_LargeDataset",
		},
		"change": {
			"BenchmarkChangeTracker_Add",
			"BenchmarkChangeTracker_Update",
			"BenchmarkChangeTracker_GetChanges",
			"BenchmarkChangeTracker_DetectChanges",
			"BenchmarkChangeTracker_AcceptChanges",
			"BenchmarkChangeTracker_MemoryUsage",
			"BenchmarkChangeTracker_LargeNumberOfEntities",
		},
		"query": {
			"BenchmarkQueryCompiler_Compile",
			"BenchmarkQueryCompiler_CompileWithCache",
			"BenchmarkQueryCompiler_CompileInsert",
			"BenchmarkQueryCompiler_CompileUpdate",
			"BenchmarkQueryCompiler_CompileDelete",
			"BenchmarkQueryCompiler_CachePerformance",
			"BenchmarkBatchCompiler_Performance",
		},
		"metadata": {
			"BenchmarkMetadataRegistry_GetOrCreate",
			"BenchmarkMetadataRegistry_GetOrCreateCached",
			"BenchmarkMetadataExtraction_ComplexEntity",
			"BenchmarkParseGoEFTags_Simple",
			"BenchmarkParseGoEFTags_Complex",
			"BenchmarkToSnakeCase",
			"BenchmarkMetadataRegistry_Memory",
		},
		"dialect": {
			"BenchmarkPostgreSQLDialect_BuildSelect",
			"BenchmarkMySQLDialect_BuildSelect",
			"BenchmarkSQLiteDialect_BuildSelect",
			"BenchmarkDialect_BuildInsert",
			"BenchmarkDialect_BuildUpdate",
			"BenchmarkDialect_MapGoTypeToSQL",
		},
		"load": {
			"TestLoad_ConcurrentUsers",
			"TestLoad_MemoryUsage",
			"TestLoad_LongRunningSession",
		},
	}

	var testsToRun []string
	if benchmarkType == "all" {
		for _, tests := range benchmarks {
			testsToRun = append(testsToRun, tests...)
		}
	} else if tests, exists := benchmarks[benchmarkType]; exists {
		testsToRun = tests
	} else {
		fmt.Printf("Unknown benchmark type: %s\n", benchmarkType)
		os.Exit(1)
	}

	fmt.Printf("Running %s benchmarks...\n", benchmarkType)
	fmt.Println("=" + strings.Repeat("=", 50))

	for _, test := range testsToRun {
		runBenchmark(test)
	}

	fmt.Println("All benchmarks completed!")
}

func runBenchmark(benchmarkName string) {
	fmt.Printf("Running %s...\n", benchmarkName)

	var cmd *exec.Cmd
	if strings.HasPrefix(benchmarkName, "Test") {
		// Run as test
		cmd = exec.Command("go", "test", "-v", "-run", benchmarkName, "./...")
	} else {
		// Run as benchmark
		cmd = exec.Command("go", "test", "-bench", benchmarkName, "-benchmem", "./...")
	}

	cmd.Dir = "../.."

	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("❌ %s failed in %v: %v\n", benchmarkName, duration, err)
		fmt.Printf("Output: %s\n", string(output))
	} else {
		fmt.Printf("✅ %s completed in %v\n", benchmarkName, duration)

		// Parse and display key metrics
		parseAndDisplayMetrics(benchmarkName, string(output))
	}

	fmt.Println(strings.Repeat("-", 50))
}

func parseAndDisplayMetrics(benchmarkName, output string) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "ns/op") || strings.Contains(line, "B/op") || strings.Contains(line, "allocs/op") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				iterations := parts[1]
				nsPerOp := parts[2]

				fmt.Printf("  📊 %s iterations, %s ns/op", iterations, nsPerOp)

				// Look for memory stats
				for i, part := range parts {
					if part == "B/op" && i > 0 {
						fmt.Printf(", %s B/op", parts[i-1])
					}
					if part == "allocs/op" && i > 0 {
						fmt.Printf(", %s allocs/op", parts[i-1])
					}
				}
				fmt.Println()
			}
		}

		// Parse custom metrics from load tests
		if strings.Contains(line, "Ops/sec:") {
			fmt.Printf("  🚀 %s\n", line)
		}
		if strings.Contains(line, "Memory used:") {
			fmt.Printf("  💾 %s\n", line)
		}
	}
}
