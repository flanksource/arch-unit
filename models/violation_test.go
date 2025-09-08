package models

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Violation", func() {
	Describe("Pretty", func() {
		It("should format a basic violation with style", func() {
			violation := Violation{
				File:          "main.go",
				Line:          10,
				Column:        5,
				CallerMethod:  "doSomething",
				CalledPackage: "forbidden.pkg",
				CalledMethod:  "BadMethod",
			}

			result := violation.Pretty()

			Expect(result.Content).To(Equal("❌ main.go:10:5: doSomething calls forbidden forbidden.pkg.BadMethod"))
			Expect(result.Style).To(Equal("text-red-600"))
		})

		It("should handle violation with only package name (no method)", func() {
			violation := Violation{
				File:          "test.go",
				Line:          20,
				Column:        15,
				CallerMethod:  "myFunction",
				CalledPackage: "forbidden.pkg",
			}

			result := violation.Pretty()

			Expect(result.Content).To(Equal("❌ test.go:20:15: myFunction calls forbidden forbidden.pkg"))
			Expect(result.Style).To(Equal("text-red-600"))
		})

		It("should include rule information when available", func() {
			rule := &Rule{
				OriginalLine: "no calls to database from controllers",
				SourceFile:   "arch.md",
				LineNumber:   5,
			}

			violation := Violation{
				File:          "controller.go",
				Line:          30,
				Column:        8,
				CallerMethod:  "HandleRequest",
				CalledPackage: "database",
				CalledMethod:  "Query",
				Rule:          rule,
			}

			result := violation.Pretty()

			expected := "❌ controller.go:30:8: HandleRequest calls forbidden database.Query (rule: no calls to database from controllers)"
			Expect(result.Content).To(Equal(expected))
			Expect(result.Style).To(Equal("text-red-600"))
		})

		It("should include message when available", func() {
			violation := Violation{
				File:          "service.go",
				Line:          45,
				Column:        12,
				CallerMethod:  "ProcessData",
				CalledPackage: "external.api",
				CalledMethod:  "Call",
				Message:       "Direct external API calls are not allowed",
			}

			result := violation.Pretty()

			expected := "❌ service.go:45:12: ProcessData calls forbidden external.api.Call - Direct external API calls are not allowed"
			Expect(result.Content).To(Equal(expected))
			Expect(result.Style).To(Equal("text-red-600"))
		})

		It("should handle violation with both rule and message", func() {
			rule := &Rule{
				OriginalLine: "services should not call external APIs directly",
				SourceFile:   "rules.md",
				LineNumber:   12,
			}

			violation := Violation{
				File:          "service.go",
				Line:          60,
				Column:        20,
				CallerMethod:  "FetchData",
				CalledPackage: "http.client",
				CalledMethod:  "Get",
				Rule:          rule,
				Message:       "Use the gateway service instead",
			}

			result := violation.Pretty()

			expected := "❌ service.go:60:20: FetchData calls forbidden http.client.Get (rule: services should not call external APIs directly) - Use the gateway service instead"
			Expect(result.Content).To(Equal(expected))
			Expect(result.Style).To(Equal("text-red-600"))
		})

		It("should handle rule with empty OriginalLine", func() {
			rule := &Rule{
				OriginalLine: "",
				SourceFile:   "rules.md",
				LineNumber:   15,
			}

			violation := Violation{
				File:          "handler.go",
				Line:          25,
				Column:        3,
				CallerMethod:  "Handle",
				CalledPackage: "forbidden",
				CalledMethod:  "Method",
				Rule:          rule,
			}

			result := violation.Pretty()

			expected := "❌ handler.go:25:3: Handle calls forbidden forbidden.Method"
			Expect(result.Content).To(Equal(expected))
			Expect(result.Style).To(Equal("text-red-600"))
		})
	})

	Describe("String", func() {
		It("should maintain existing string format for backward compatibility", func() {
			rule := &Rule{
				OriginalLine: "test rule",
				SourceFile:   "test.md",
				LineNumber:   1,
			}

			violation := Violation{
				File:          "test.go",
				Line:          10,
				Column:        5,
				CallerMethod:  "TestMethod",
				CalledPackage: "pkg",
				CalledMethod:  "Method",
				Rule:          rule,
			}

			result := violation.String()
			expected := "test.go:10:5: TestMethod calls forbidden pkg.Method (rule: test rule in test.md:1)"
			Expect(result).To(Equal(expected))
		})
	})
})
