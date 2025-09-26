package com.example.service;

import java.util.List;
import java.util.ArrayList;
import java.io.IOException;
import java.sql.SQLException;

/**
 * UserService demonstrates Java 1.7 features
 */
public class UserService extends BaseService implements ServiceInterface {

    private static final String DEFAULT_NAME = "Unknown";
    private final List<String> userNames = new ArrayList<>(); // Diamond operator

    public UserService() {
        super();
    }

    public UserService(String defaultName) {
        super(defaultName);
    }

    // Try-with-resources (Java 1.7)
    public List<String> readUsersFromFile(String filename) throws IOException {
        try (java.io.BufferedReader reader = new java.io.BufferedReader(
                new java.io.FileReader(filename))) {

            List<String> users = new ArrayList<>();
            String line;
            while ((line = reader.readLine()) != null) {
                users.add(line.trim());
            }
            return users;
        }
    }

    // Multi-catch (Java 1.7)
    public void processUser(String userData) {
        try {
            parseUserData(userData);
            validateUser(userData);
        } catch (IOException | SQLException ex) {
            handleException(ex);
        }
    }

    // String in switch (Java 1.7)
    public int getUserType(String userRole) {
        switch (userRole) {
            case "ADMIN":
                return 1;
            case "USER":
                return 2;
            case "GUEST":
                return 3;
            default:
                return 0;
        }
    }

    // Binary literals and underscores (Java 1.7)
    public void calculateMetrics() {
        int binaryValue = 0b1010_1100; // Binary literal with underscores
        long largeNumber = 1_000_000L; // Underscores in numeric literals

        if (isValidMetric(binaryValue)) {
            updateMetrics(largeNumber);
        }
    }

    @Override
    public void performService() {
        System.out.println("Performing user service");
    }

    private void parseUserData(String data) throws IOException {
        if (data == null || data.isEmpty()) {
            throw new IOException("Invalid user data");
        }
    }

    private void validateUser(String data) throws SQLException {
        if (data.length() < 3) {
            throw new SQLException("User data too short");
        }
    }

    private void handleException(Exception ex) {
        System.err.println("Error processing user: " + ex.getMessage());
    }

    private boolean isValidMetric(int value) {
        return value > 0;
    }

    private void updateMetrics(long value) {
        // Update metrics logic
    }
}