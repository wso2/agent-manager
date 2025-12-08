# Database connection is now handled by setup.db_config
# Import the connection function
from setup.db_config import get_db_connection

# For backward compatibility, keep db as None
# Tools should use get_db_connection() instead
db = None
