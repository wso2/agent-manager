from functools import lru_cache

from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    hotel_api_token: SecretStr = Field(min_length=1)
    pinecone_api_key: str | None = Field(None, validation_alias="PINECONE_API_KEY")
    pinecone_service_url: str | None = Field(None, validation_alias="PINECONE_SERVICE_URL")
    pinecone_index_name: str = Field("hotel-policies", validation_alias="PINECONE_INDEX_NAME")
    openai_api_key: str | None = Field(None, validation_alias="OPENAI_API_KEY")
    openai_embedding_model: str = Field(
        default="text-embedding-3-small",
        validation_alias="OPENAI_EMBEDDING_MODEL",
    )
    policies_dirs: str | None = Field(default=None, validation_alias="POLICIES_DIRS")

    model_config = SettingsConfigDict(env_file=".env", extra="ignore", hide_input_in_errors=True)


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()
