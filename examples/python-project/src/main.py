#!/usr/bin/env python3

import logging
from internal.config import Config  # VIOLATION: accessing internal module
from api.server import Server
from service.user_service import UserService

def main():
    # VIOLATION: Using print instead of logging
    print("Starting application...")
    
    # This is fine - using logger
    logging.info("Application started")
    
    # VIOLATION: Accessing internal module
    config = Config.load()
    
    # This is fine - using public modules
    server = Server()
    service = UserService()
    
    server.start(service)

if __name__ == "__main__":
    main()