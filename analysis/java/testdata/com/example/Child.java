package com.example;



public class Child extends Parent implements Service {
    public void childMethod() {
        parentMethod();
    }

    public void serviceMethod() {
    }
}
