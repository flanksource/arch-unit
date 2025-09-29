package com.example;

import java.util.List;

// Public class - should be public (not private)
public class VisibilityTestClass {

    // Private field - should be private
    private String privateField;

    // Public field - should not be private
    public int publicField;

    // Package-private field - should not be private (Java considers package-private as not private)
    String packageField;

    // Protected field - should not be private
    protected double protectedField;

    // Private static field - should be private
    private static final String PRIVATE_CONSTANT = "secret";

    // Public static field - should not be private
    public static final String PUBLIC_CONSTANT = "visible";

    // Private constructor - should be private
    private VisibilityTestClass(String secret) {
        this.privateField = secret;
    }

    // Public constructor - should not be private
    public VisibilityTestClass() {
        this.privateField = "default";
    }

    // Package-private constructor - should not be private
    VisibilityTestClass(int value) {
        this.publicField = value;
    }

    // Private method - should be private
    private void privateMethod() {
        System.out.println("This is private");
    }

    // Public method - should not be private
    public String getPrivateField() {
        return privateField;
    }

    // Package-private method - should not be private
    void packageMethod() {
        System.out.println("Package visible");
    }

    // Protected method - should not be private
    protected void protectedMethod() {
        System.out.println("Protected method");
    }

    // Private static method - should be private
    private static String getSecret() {
        return PRIVATE_CONSTANT;
    }

    // Public static method - should not be private
    public static String getPublicConstant() {
        return PUBLIC_CONSTANT;
    }
}

// Package-private class - should not be private in Java context
class PackagePrivateClass {
    private String data;
    public String info;

    private void secretMethod() {}
    public void publicMethod() {}
    void packageMethod() {}
}

// Private nested class context
class OuterClass {
    // Private nested class - should be private
    private static class PrivateNestedClass {
        private String nestedPrivateField;
        public String nestedPublicField;

        private void nestedPrivateMethod() {}
        public void nestedPublicMethod() {}
    }

    // Public nested class - should not be private
    public static class PublicNestedClass {
        private String nestedPrivateField;
        public String nestedPublicField;

        private void nestedPrivateMethod() {}
        public void nestedPublicMethod() {}
    }
}