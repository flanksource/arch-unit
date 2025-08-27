import logging


class Config:
    def __init__(self):
        self.database_url = "sqlite:///app.db"
        self.api_key = "secret-key"
    
    @classmethod
    def load(cls):
        # This is fine - internal module using logger
        logging.info("Loading configuration...")
        
        return cls()