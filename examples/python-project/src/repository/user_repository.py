import sqlite3  # OK: Repository can access database
import logging
from typing import Optional, Dict


class UserRepository:
    def __init__(self):
        # OK: Repository can have database connection
        self.conn = sqlite3.connect('users.db')
        self.cursor = self.conn.cursor()
        self._create_table()
    
    def _create_table(self):
        # OK: Repository can execute SQL
        self.cursor.execute('''
            CREATE TABLE IF NOT EXISTS users (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL
            )
        ''')
        self.conn.commit()
    
    def get_by_id(self, user_id: str) -> Optional[Dict]:
        # OK: Repository can execute SQL queries
        self.cursor.execute("SELECT id, name FROM users WHERE id = ?", (user_id,))
        
        # This is fine - using logger
        logging.info(f"Fetching user from database: {user_id}")
        
        row = self.cursor.fetchone()
        if row:
            return {"id": row[0], "name": row[1]}
        return None
    
    def create(self, user_id: str, name: str) -> bool:
        # OK: Repository can execute SQL statements
        try:
            self.cursor.execute(
                "INSERT INTO users (id, name) VALUES (?, ?)",
                (user_id, name)
            )
            self.conn.commit()
            return True
        except sqlite3.IntegrityError:
            logging.error(f"User {user_id} already exists")
            return False