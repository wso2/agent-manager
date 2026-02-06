from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    pinecone_api_key: str = Field(..., validation_alias="PINECONE_API_KEY")
    pinecone_service_url: str = Field(..., validation_alias="PINECONE_SERVICE_URL")
    pinecone_index_name: str = Field(..., validation_alias="PINECONE_INDEX_NAME")
    openai_embedding_model: str = Field(
        default="text-embedding-3-small",
        validation_alias="OPENAI_EMBEDDING_MODEL",
    )
    policies_dirs: str | None = Field(default=None, validation_alias="POLICIES_DIRS")

    model_config = SettingsConfigDict(env_file=".env", extra="ignore")


settings = Settings()
