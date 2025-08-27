import sqlite3  # VIOLATION: Direct database access outside repository
import logging
import requests  # VIOLATION: Direct HTTP client usage
import unittest  # VIOLATION: Test module in production


class UserService:
    def __init__(self):
        # VIOLATION: Direct database connection
        self.conn = sqlite3.connect('users.db')
        self.cursor = self.conn.cursor()
    
    def get_user(self, user_id):
        # VIOLATION: Direct database query outside repository
        self.cursor.execute("SELECT * FROM users WHERE id = ?", (user_id,))
        
        # VIOLATION: Using print
        print(f"Fetching user {user_id}")
        
        # This is fine - using logger
        logging.info(f"User fetch requested for ID: {user_id}")
        
        # VIOLATION: Direct HTTP call
        response = requests.get(f"https://api.example.com/user/{user_id}")
        
        return {"id": user_id, "name": "John Doe"}
    
    # VIOLATION: Test method in production code
    def test_user_creation(self):
        """Test method shouldn't be in production"""
        assert self.get_user("123") is not None
    
    # VIOLATION: Using unittest in production
    def run_tests(self):
        suite = unittest.TestSuite()
        return suite