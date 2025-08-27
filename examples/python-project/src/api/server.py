import logging
from service.user_service import UserService


class Server:
    def __init__(self):
        self.service = None
    
    def start(self, service: UserService):
        self.service = service
        
        # This is fine - using logger
        logging.info("API server starting...")
        
        # Start HTTP server
        self.setup_routes()
    
    def setup_routes(self):
        # Route setup logic
        logging.info("Routes configured")
        
    def handle_get_user(self, user_id: str):
        # This is fine - server can use service
        user = self.service.get_user(user_id)
        return user