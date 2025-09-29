package com.flanksource.archunit;

import com.github.javaparser.JavaParser;
import com.github.javaparser.ParseResult;
import com.github.javaparser.ast.CompilationUnit;
import com.github.javaparser.ast.ImportDeclaration;
import com.github.javaparser.ast.Modifier;
import com.github.javaparser.ast.Node;
import com.github.javaparser.ast.body.*;
import com.github.javaparser.ast.expr.MethodCallExpr;
import com.github.javaparser.ast.stmt.BlockStmt;
import com.github.javaparser.ast.type.Type;
import com.github.javaparser.ast.visitor.VoidVisitorAdapter;
import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.annotations.SerializedName;

import java.io.File;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.*;

/**
 * Java AST Extractor for Java 1.7 compatibility
 * Uses JavaParser to extract AST information and output as JSON
 */
public class JavaASTExtractor {

    private final Gson gson;
    private String packageName = "";
    private String currentClass = "";
    private String currentFilePath = "";
    private final List<GoASTNode> nodes = new ArrayList<GoASTNode>();
    private final List<ASTImport> imports = new ArrayList<ASTImport>();
    private final List<ASTRelationship> relationships = new ArrayList<ASTRelationship>();

    public JavaASTExtractor() {
        this.gson = new GsonBuilder().setPrettyPrinting().create();
    }

    public static void main(String[] args) {
        if (args.length != 1) {
            System.err.println("Usage: java JavaASTExtractor <java-file>");
            System.exit(1);
        }

        String filePath = args[0];
        JavaASTExtractor extractor = new JavaASTExtractor();

        try {
            ASTResult result = extractor.extractAST(filePath);
            System.out.println(extractor.gson.toJson(result));
        } catch (Exception e) {
            System.err.println("Error parsing file: " + e.getMessage());
            e.printStackTrace();
            System.exit(1);
        }
    }

    public ASTResult extractAST(String filePath) throws IOException {
        File file = new File(filePath);
        if (!file.exists()) {
            throw new IOException("File does not exist: " + filePath);
        }

        // Store file path for use in nodes
        this.currentFilePath = filePath;

        String content = new String(Files.readAllBytes(Paths.get(filePath)), "UTF-8");

        JavaParser parser = new JavaParser();
        ParseResult<CompilationUnit> parseResult = parser.parse(content);

        if (!parseResult.isSuccessful()) {
            throw new RuntimeException("Failed to parse Java file: " + parseResult.getProblems());
        }

        CompilationUnit cu = parseResult.getResult().get();

        // Extract package name
        if (cu.getPackageDeclaration().isPresent()) {
            packageName = cu.getPackageDeclaration().get().getNameAsString();
        }

        // Clear collections for fresh extraction
        nodes.clear();
        imports.clear();
        relationships.clear();

        // Extract imports
        extractImports(cu);

        // Extract nodes using visitor pattern
        ASTNodeVisitor visitor = new ASTNodeVisitor();
        cu.accept(visitor, null);

        // Create and return result
        ASTResult result = new ASTResult();
        result.nodes = nodes;
        result.imports = imports;
        result.relationships = relationships;
        result.packageName = packageName;
        result.className = currentClass;

        return result;
    }

    private void extractImports(CompilationUnit cu) {
        for (ImportDeclaration imp : cu.getImports()) {
            ASTImport astImport = new ASTImport();
            astImport.source = imp.getNameAsString();
            astImport.line = imp.getBegin().isPresent() ? imp.getBegin().get().line : 0;
            astImport.isStatic = imp.isStatic();
            astImport.isWild = imp.isAsterisk();
            imports.add(astImport);
        }
    }

    /**
     * Visitor class to traverse the AST and extract nodes
     */
    private class ASTNodeVisitor extends VoidVisitorAdapter<Void> {

        @Override
        public void visit(ClassOrInterfaceDeclaration n, Void arg) {
            currentClass = n.getNameAsString();

            GoASTNode node = new GoASTNode();

            // Set Go-compatible fields
            node.filePath = currentFilePath;
            node.packageName = packageName;
            node.typeName = n.getNameAsString();
            node.nodeType = "type"; // Go uses "type" for classes/interfaces
            node.startLine = n.getBegin().isPresent() ? n.getBegin().get().line : 0;
            node.endLine = n.getEnd().isPresent() ? n.getEnd().get().line : 0;
            node.lineCount = node.endLine - node.startLine + 1;

            // Set visibility - Go only tracks isPrivate
            node.isPrivate = n.hasModifier(Modifier.Keyword.PRIVATE);

            // No parameters or returns for types
            node.parameterCount = 0;
            node.returnCount = 0;
            node.cyclomaticComplexity = 0;

            // Extract extends/implements relationships
            if (!n.getExtendedTypes().isEmpty()) {
                String superClass = n.getExtendedTypes().get(0).getNameAsString();

                // Create extends relationship
                ASTRelationship relationship = new ASTRelationship();
                relationship.fromNode = getFullName(currentClass);
                relationship.toNode = superClass;
                relationship.type = "extends";
                relationship.line = node.startLine;
                relationships.add(relationship);
            }

            if (!n.getImplementedTypes().isEmpty()) {
                for (Type type : n.getImplementedTypes()) {
                    String interfaceName = type.asString();

                    // Create implements relationship
                    ASTRelationship relationship = new ASTRelationship();
                    relationship.fromNode = getFullName(currentClass);
                    relationship.toNode = interfaceName;
                    relationship.type = "implements";
                    relationship.line = node.startLine;
                    relationships.add(relationship);
                }
            }

            nodes.add(node);
            super.visit(n, arg);
        }

        @Override
        public void visit(EnumDeclaration n, Void arg) {
            currentClass = n.getNameAsString();

            GoASTNode node = new GoASTNode();

            // Set Go-compatible fields
            node.filePath = currentFilePath;
            node.packageName = packageName;
            node.typeName = n.getNameAsString();
            node.nodeType = "type"; // Go treats enums as types
            node.startLine = n.getBegin().isPresent() ? n.getBegin().get().line : 0;
            node.endLine = n.getEnd().isPresent() ? n.getEnd().get().line : 0;
            node.lineCount = node.endLine - node.startLine + 1;

            // Set visibility
            node.isPrivate = n.hasModifier(Modifier.Keyword.PRIVATE);

            // No parameters or returns for enums
            node.parameterCount = 0;
            node.returnCount = 0;
            node.cyclomaticComplexity = 0;

            nodes.add(node);
            super.visit(n, arg);
        }

        @Override
        public void visit(MethodDeclaration n, Void arg) {
            GoASTNode node = new GoASTNode();

            // Set Go-compatible fields
            node.filePath = currentFilePath;
            node.packageName = packageName;
            node.typeName = currentClass;
            node.methodName = n.getNameAsString();
            node.nodeType = "method";
            node.startLine = n.getBegin().isPresent() ? n.getBegin().get().line : 0;
            node.endLine = n.getEnd().isPresent() ? n.getEnd().get().line : 0;
            node.lineCount = node.endLine - node.startLine + 1;

            // Set visibility
            node.isPrivate = n.hasModifier(Modifier.Keyword.PRIVATE);

            // Extract detailed parameter information
            node.parameterCount = n.getParameters().size();
            for (com.github.javaparser.ast.body.Parameter param : n.getParameters()) {
                String paramName = param.getNameAsString();
                String paramType = param.getType().asString();
                node.parameters.add(new Parameter(paramName, paramType));
            }

            // Extract return value information
            if (n.getType().isVoidType()) {
                node.returnCount = 0;
            } else {
                node.returnCount = 1;
                String returnType = n.getType().asString();
                node.returnValues.add(new ReturnValue("", returnType));
            }

            // Calculate cyclomatic complexity
            node.cyclomaticComplexity = calculateCyclomaticComplexity(n);

            nodes.add(node);

            // Extract method calls
            extractMethodCalls(n);

            super.visit(n, arg);
        }

        @Override
        public void visit(ConstructorDeclaration n, Void arg) {
            GoASTNode node = new GoASTNode();

            // Set Go-compatible fields
            node.filePath = currentFilePath;
            node.packageName = packageName;
            node.typeName = currentClass;
            node.methodName = n.getNameAsString();
            node.nodeType = "method"; // Go treats constructors as methods
            node.startLine = n.getBegin().isPresent() ? n.getBegin().get().line : 0;
            node.endLine = n.getEnd().isPresent() ? n.getEnd().get().line : 0;
            node.lineCount = node.endLine - node.startLine + 1;

            // Set visibility
            node.isPrivate = n.hasModifier(Modifier.Keyword.PRIVATE);

            // Extract detailed parameter information
            node.parameterCount = n.getParameters().size();
            for (com.github.javaparser.ast.body.Parameter param : n.getParameters()) {
                String paramName = param.getNameAsString();
                String paramType = param.getType().asString();
                node.parameters.add(new Parameter(paramName, paramType));
            }

            // Constructors don't return values
            node.returnCount = 0;

            // Calculate cyclomatic complexity
            node.cyclomaticComplexity = calculateCyclomaticComplexity(n);

            nodes.add(node);

            // Extract method calls
            extractMethodCalls(n);

            super.visit(n, arg);
        }

        @Override
        public void visit(FieldDeclaration n, Void arg) {
            for (VariableDeclarator variable : n.getVariables()) {
                GoASTNode node = new GoASTNode();

                // Set Go-compatible fields
                node.filePath = currentFilePath;
                node.packageName = packageName;
                node.typeName = currentClass;
                node.fieldName = variable.getNameAsString();
                node.nodeType = "field";
                node.startLine = n.getBegin().isPresent() ? n.getBegin().get().line : 0;
                node.endLine = n.getEnd().isPresent() ? n.getEnd().get().line : 0;
                node.lineCount = node.endLine - node.startLine + 1;

                // Set visibility
                node.isPrivate = n.hasModifier(Modifier.Keyword.PRIVATE);

                // Fields don't have parameters, returns, or complexity
                node.parameterCount = 0;
                node.returnCount = 0;
                node.cyclomaticComplexity = 0;

                nodes.add(node);
            }
            super.visit(n, arg);
        }

        private void extractMethodCalls(final CallableDeclaration<?> method) {
            method.accept(new VoidVisitorAdapter<Void>() {
                @Override
                public void visit(MethodCallExpr n, Void arg) {
                    // Create a "calls" relationship
                    ASTRelationship relationship = new ASTRelationship();
                    relationship.fromNode = getFullName(currentClass, method.getNameAsString());
                    relationship.toNode = n.getNameAsString();
                    relationship.type = "calls";
                    relationship.line = n.getBegin().isPresent() ? n.getBegin().get().line : 0;
                    relationships.add(relationship);

                    super.visit(n, arg);
                }
            }, null);
        }
    }


    private int calculateCyclomaticComplexity(CallableDeclaration<?> method) {
        final int[] complexity = {1}; // Base complexity is 1

        method.accept(new VoidVisitorAdapter<Void>() {
            @Override
            public void visit(BlockStmt n, Void arg) {
                // This is a simplified complexity calculation
                // In a full implementation, you'd count decision points like if, while, for, switch, etc.
                String blockStr = n.toString();
                complexity[0] += countOccurrences(blockStr, "if ");
                complexity[0] += countOccurrences(blockStr, "while ");
                complexity[0] += countOccurrences(blockStr, "for ");
                complexity[0] += countOccurrences(blockStr, "switch ");
                complexity[0] += countOccurrences(blockStr, "catch ");
                complexity[0] += countOccurrences(blockStr, "case ");
                super.visit(n, arg);
            }
        }, null);

        return complexity[0];
    }

    private int countOccurrences(String text, String pattern) {
        int count = 0;
        int index = 0;
        while ((index = text.indexOf(pattern, index)) != -1) {
            count++;
            index += pattern.length();
        }
        return count;
    }

    private String getFullName(String className) {
        if (packageName.isEmpty()) {
            return className;
        }
        return packageName + "." + className;
    }

    private String getFullName(String className, String methodName) {
        if (packageName.isEmpty()) {
            return className + "." + methodName;
        }
        return packageName + "." + className + "." + methodName;
    }

    // Data classes for JSON serialization - now matches Go models.ASTNode structure
    public static class ASTResult {
        public List<GoASTNode> nodes;
        public List<ASTImport> imports;
        public List<ASTRelationship> relationships;
        public String packageName;
        public String className;
    }

    public static class GoASTNode {
        // Core identification fields - using snake_case to match Go JSON tags
        @SerializedName("file_path")
        public String filePath;
        @SerializedName("package_name")
        public String packageName;
        @SerializedName("type_name")
        public String typeName;
        @SerializedName("method_name")
        public String methodName;
        @SerializedName("field_name")
        public String fieldName;
        @SerializedName("node_type")
        public String nodeType;
        public String language;

        // Location information
        @SerializedName("start_line")
        public int startLine;
        @SerializedName("end_line")
        public int endLine;
        @SerializedName("line_count")
        public int lineCount;

        // Complexity and metrics
        @SerializedName("cyclomatic_complexity")
        public int cyclomaticComplexity;
        @SerializedName("parameter_count")
        public int parameterCount;
        @SerializedName("return_count")
        public int returnCount;

        // Detailed parameter and return information
        public List<Parameter> parameters;
        @SerializedName("return_values")
        public List<ReturnValue> returnValues;

        // Visibility
        @SerializedName("is_private")
        public boolean isPrivate;

        // Constructor for easy creation
        public GoASTNode() {
            this.parameters = new ArrayList<Parameter>();
            this.returnValues = new ArrayList<ReturnValue>();
            this.language = "java";
        }
    }

    public static class ASTImport {
        public String source;
        public int line;
        public boolean isStatic;
        public boolean isWild;
    }

    public static class ASTRelationship {
        public String fromNode;
        public String toNode;
        public String type;
        public int line;
    }

    public static class Parameter {
        public String name;
        public String type;
        @SerializedName("name_length")
        public int nameLength;

        public Parameter(String name, String type) {
            this.name = name;
            this.type = type;
            this.nameLength = name != null ? name.length() : 0;
        }
    }

    public static class ReturnValue {
        public String name;
        public String type;

        public ReturnValue(String name, String type) {
            this.name = name;
            this.type = type;
        }
    }
}