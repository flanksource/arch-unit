package com.example.service;

public abstract class BaseService {
    protected String serviceName;

    public BaseService() {
        this.serviceName = "DefaultService";
    }

    public BaseService(String name) {
        this.serviceName = name;
    }

    public abstract void performService();

    protected final void logService() {
        System.out.println("Service: " + serviceName);
    }
}