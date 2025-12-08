"""
Database configuration module for PostgreSQL connection.
Supports both Neon and Supabase PostgreSQL databases.
"""
import os
from sqlalchemy import create_engine
from sqlalchemy.pool import NullPool
from dotenv import load_dotenv

load_dotenv()


def get_database_url():
    """
    Get the database URL from environment variables.
    
    You can set either:
    - DATABASE_URL: Full PostgreSQL connection string
    - Or individual components: DB_HOST, DB_NAME, DB_USER, DB_PASSWORD, DB_PORT
    
    Example for Neon:
    DATABASE_URL=postgresql://user:password@ep-xxx.region.aws.neon.tech/dbname?sslmode=require
    
    Example for Supabase:
    DATABASE_URL=postgresql://postgres:password@db.xxx.supabase.co:5432/postgres
    """
    database_url = os.getenv("DATABASE_URL")
    
    if not database_url:
        # Build URL from components
        db_host = os.getenv("DB_HOST")
        db_name = os.getenv("DB_NAME", "postgres")
        db_user = os.getenv("DB_USER", "postgres")
        db_password = os.getenv("DB_PASSWORD")
        db_port = os.getenv("DB_PORT", "5432")
        
        if not all([db_host, db_password]):
            raise ValueError(
                "Either DATABASE_URL or DB_HOST and DB_PASSWORD must be set in environment variables"
            )
        
        database_url = f"postgresql://{db_user}:{db_password}@{db_host}:{db_port}/{db_name}"
    
    # Ensure SSL mode for remote connections (required by Neon and Supabase)
    if "sslmode" not in database_url and ("neon.tech" in database_url or "supabase.co" in database_url):
        separator = "&" if "?" in database_url else "?"
        database_url += f"{separator}sslmode=require"
    
    return database_url


def create_db_engine():
    """Create and return a SQLAlchemy engine."""
    database_url = get_database_url()
    # Use NullPool for serverless environments to avoid connection pooling issues
    engine = create_engine(
        database_url,
        poolclass=NullPool,
        echo=False
    )
    return engine


def get_db_connection():
    """
    Get a raw database connection.
    Remember to close the connection after use.
    """
    engine = create_db_engine()
    return engine.raw_connection()
