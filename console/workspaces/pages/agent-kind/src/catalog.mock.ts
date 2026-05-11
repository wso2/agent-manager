export interface CatalogItemVersion {
  releaseDate: string;
  apiSpecs?: Record<string, unknown> | null;
  runtimeConfig?: Record<string, {isSecrete: boolean, type: "string" | boolean | number}> | null;
}

export interface CatalogItem {
  id: string;
  title: string;
  tags: string[];
  createdAt: string;
  description: string;
  versions: Record<string, CatalogItemVersion>;
}

export interface LatestVersion extends CatalogItemVersion {
  versionKey: string;
}

/** Returns the version entry with the latest releaseDate, including its version key. */
export function getLatestVersion(item: CatalogItem): LatestVersion | undefined {
  const sorted = Object.entries(item.versions).sort(
    ([, a], [, b]) => new Date(b.releaseDate).getTime() - new Date(a.releaseDate).getTime(),
  );
  if (sorted.length === 0) return undefined;
  const [versionKey, version] = sorted[0];
  return { ...version, versionKey };
}

const buildSpec = (title: string, version: string, inputSchema: unknown, outputSchema: unknown) => ({
  openapi: "3.0.0",
  info: { title, version },
  paths: {
    "/invoke": {
      post: {
        summary: "Invoke the agent",
        operationId: "invokeAgent",
        requestBody: {
          required: true,
          content: {
            "application/json": {
              schema: inputSchema,
            },
          },
        },
        responses: {
          "200": {
            description: "Successful response",
            content: {
              "application/json": {
                schema: outputSchema,
              },
            },
          },
        },
      },
    },
  },
});

export const DUMMY_CATALOG_LIST: CatalogItem[] = [
  {
    id: "customer-support-agent",
    title: "Customer Support Agent",
    tags: ["chat", "rag", "customerSupport", "knowledgeBase"],
    createdAt: "2024-01-01",
    description: "Handles customer queries using RAG over a knowledge base.",
    versions: {
      "1.0": {
        releaseDate: "2024-01-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Customer Support Agent", "1.0",
          {
            type: "object",
            properties: {
              query: { type: "string", description: "Customer's question or issue." },
              context: { type: "object", description: "Additional context for better answers." },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              answer: { type: "string", description: "Agent's response to the customer's query." },
              sources: {
                type: "array",
                items: { type: "string" },
                description: "List of knowledge base entries used to generate the answer.",
              },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-02-15",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Customer Support Agent", "1.1",
          {
            type: "object",
            properties: {
              query: { type: "string", description: "Customer's question or issue." },
              context: { type: "object", description: "Additional context for better answers." },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              answer: { type: "string", description: "Agent's response to the customer's query." },
              sources: {
                type: "array",
                items: { type: "string" },
                description: "List of knowledge base entries used to generate the answer.",
              },
              detected_sentiment: { type: "string", description: "Detected sentiment of the customer's query." },
            },
          }
        ),
      },
    },
  },
  {
    id: "document-retriever",
    title: "Document Retriever",
    tags: ["retriever", "vectorDB", "rag"],
    createdAt: "2024-01-15",
    description: "Retrieves and ranks relevant documents from a vector store.",
    versions: {
      "1.0": {
        releaseDate: "2024-01-15",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Document Retriever", "1.0",
          {
            type: "object",
            properties: {
              query: { type: "string", description: "Search query to retrieve relevant documents." },
              top_k: { type: "number", description: "Maximum number of documents to return. Defaults to 5." },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              documents: {
                type: "array",
                items: {
                  type: "object",
                  properties: {
                    id: { type: "string", description: "Document identifier." },
                    content: { type: "string", description: "Document content snippet." },
                    score: { type: "number", description: "Similarity score." },
                  },
                },
                description: "Ranked list of retrieved documents.",
              },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-03-10",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Document Retriever", "1.1",
          {
            type: "object",
            properties: {
              query: { type: "string", description: "Search query to retrieve relevant documents." },
              top_k: { type: "number", description: "Maximum number of documents to return. Defaults to 5." },
              search_mode: { type: "string", description: "Search strategy: 'vector', 'bm25', or 'hybrid'." },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              documents: {
                type: "array",
                items: {
                  type: "object",
                  properties: {
                    id: { type: "string" },
                    content: { type: "string" },
                    vector_score: { type: "number" },
                    bm25_score: { type: "number" },
                    final_score: { type: "number" },
                  },
                },
              },
            },
          }
        ),
      },
    },
  },
  {
    id: "code-assistant",
    title: "Code Assistant",
    tags: ["code", "assistant", "developer"],
    createdAt: "2024-02-01",
    description: "Assists developers with code generation and reviews.",
    versions: {
      "1.0": {
        releaseDate: "2024-02-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Code Assistant", "1.0",
          {
            type: "object",
            properties: {
              prompt: { type: "string", description: "Code generation or review instruction." },
              language: { type: "string", description: "Target programming language." },
            },
            required: ["prompt"],
          },
          {
            type: "object",
            properties: {
              code: { type: "string", description: "Generated or reviewed code snippet." },
              explanation: { type: "string", description: "Explanation of the generated code." },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-04-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Code Assistant", "1.1",
          {
            type: "object",
            properties: {
              prompt: { type: "string", description: "Code generation instruction." },
              language: { type: "string", description: "Target programming language." },
              task: { type: "string", description: "Task type: generate, review, or test." },
            },
            required: ["prompt"],
          },
          {
            type: "object",
            properties: {
              code: { type: "string" },
              explanation: { type: "string" },
              tests: { type: "string" },
            },
          }
        ),
      },
    },
  },
  {
    id: "hr-policy-bot",
    title: "HR Policy Bot",
    tags: ["chat", "hr", "knowledgeBase"],
    createdAt: "2024-02-14",
    description: "Answers employee questions about HR policies and benefits.",
    versions: {
      "1.0": {
        releaseDate: "2024-02-14",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("HR Policy Bot", "1.0",
          {
            type: "object",
            properties: {
              question: { type: "string", description: "Employee's HR policy question." },
            },
            required: ["question"],
          },
          {
            type: "object",
            properties: {
              answer: { type: "string" },
              policy_references: {
                type: "array",
                items: { type: "string" },
              },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-04-20",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("HR Policy Bot", "1.1",
          {
            type: "object",
            properties: {
              question: { type: "string" },
              role: { type: "string", description: "Employee role for filtering." },
              region: { type: "string", description: "Employee region for lookup." },
            },
            required: ["question"],
          },
          {
            type: "object",
            properties: {
              answer: { type: "string" },
              policy_references: { type: "array", items: { type: "string" } },
              applicable_regions: { type: "array", items: { type: "string" } },
            },
          }
        ),
      },
    },
  },
  {
    id: "sales-intelligence-agent",
    title: "Sales Intelligence Agent",
    tags: ["analytics", "sales", "insights"],
    createdAt: "2024-03-01",
    description: "Analyzes sales data and provides actionable insights.",
    versions: {
      "1.0": {
        releaseDate: "2024-03-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Sales Intelligence Agent", "1.0",
          {
            type: "object",
            properties: {
              query: { type: "string" },
              time_range: { type: "object" },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              insights: { type: "array", items: { type: "string" } },
              summary: { type: "string" },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-05-05",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Sales Intelligence Agent", "1.1",
          {
            type: "object",
            properties: {
              query: { type: "string" },
              time_range: { type: "object" },
              include_forecast: { type: "boolean" },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              insights: { type: "array", items: { type: "string" } },
              summary: { type: "string" },
              forecast: { type: "object" },
              competitor_benchmarks: { type: "array" },
            },
          }
        ),
      },
    },
  },
  {
    id: "legal-document-summarizer",
    title: "Legal Document Summarizer",
    tags: ["summarization", "legal", "rag"],
    createdAt: "2024-03-20",
    description: "Summarizes lengthy legal documents into concise briefs.",
    versions: {
      "1.0": {
        releaseDate: "2024-03-20",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Legal Document Summarizer", "1.0",
          {
            type: "object",
            properties: {
              document: { type: "string" },
              max_length: { type: "number" },
            },
            required: ["document"],
          },
          {
            type: "object",
            properties: {
              summary: { type: "string" },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-05-15",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Legal Document Summarizer", "1.1",
          {
            type: "object",
            properties: {
              document: { type: "string" },
              max_length: { type: "number" },
              extract_clauses: { type: "boolean" },
            },
            required: ["document"],
          },
          {
            type: "object",
            properties: {
              summary: { type: "string" },
              clauses: { type: "array", items: { type: "object" } },
              risk_flags: { type: "array", items: { type: "string" } },
            },
          }
        ),
      },
    },
  },
  {
    id: "travel-booking-assistant",
    title: "Travel Booking Assistant",
    tags: ["chat", "travel", "booking"],
    createdAt: "2024-04-05",
    description: "Helps users plan and book travel itineraries.",
    versions: {
      "1.0": {
        releaseDate: "2024-04-05",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Travel Booking Assistant", "1.0",
          {
            type: "object",
            properties: {
              destination: { type: "string" },
              travel_dates: { type: "object" },
              preferences: { type: "object" },
            },
            required: ["destination", "travel_dates"],
          },
          {
            type: "object",
            properties: {
              itinerary: { type: "string" },
              flight_options: { type: "array" },
              hotel_options: { type: "array" },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-06-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Travel Booking Assistant", "1.1",
          {
            type: "object",
            properties: {
              destination: { type: "string" },
              travel_dates: { type: "object" },
              preferences: { type: "object" },
              nationality: { type: "string" },
            },
            required: ["destination", "travel_dates"],
          },
          {
            type: "object",
            properties: {
              itinerary: { type: "string" },
              flight_options: { type: "array" },
              hotel_options: { type: "array" },
              visa_requirements: { type: "object" },
              local_recommendations: { type: "array" },
            },
          }
        ),
      },
    },
  },
  {
    id: "medical-faq-agent",
    title: "Medical FAQ Agent",
    tags: ["chat", "medical", "knowledgeBase", "rag"],
    createdAt: "2024-04-18",
    description: "Answers frequently asked medical questions from verified sources.",
    versions: {
      "1.0": {
        releaseDate: "2024-04-18",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Medical FAQ Agent", "1.0",
          {
            type: "object",
            properties: {
              question: { type: "string" },
            },
            required: ["question"],
          },
          {
            type: "object",
            properties: {
              answer: { type: "string" },
              disclaimer: { type: "string" },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-06-10",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Medical FAQ Agent", "1.1",
          {
            type: "object",
            properties: {
              question: { type: "string" },
              symptoms: { type: "array", items: { type: "string" } },
            },
            required: ["question"],
          },
          {
            type: "object",
            properties: {
              answer: { type: "string" },
              disclaimer: { type: "string" },
              sources: { type: "array", items: { type: "string" } },
              triage_guidance: { type: "string" },
            },
          }
        ),
      },
    },
  },
  {
    id: "ecommerce-product-advisor",
    title: "E-commerce Product Advisor",
    tags: ["recommendation", "ecommerce", "personalization"],
    createdAt: "2024-05-01",
    description: "Recommends products based on user preferences and history.",
    versions: {
      "1.0": {
        releaseDate: "2024-05-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("E-commerce Product Advisor", "1.0",
          {
            type: "object",
            properties: {
              user_id: { type: "string" },
              preferences: { type: "object" },
            },
            required: ["user_id"],
          },
          {
            type: "object",
            properties: {
              recommendations: {
                type: "array",
                items: {
                  type: "object",
                  properties: {
                    product_id: { type: "string" },
                    score: { type: "number" },
                    reason: { type: "string" },
                  },
                },
              },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-07-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("E-commerce Product Advisor", "1.1",
          {
            type: "object",
            properties: {
              user_id: { type: "string" },
              preferences: { type: "object" },
              context: { type: "object" },
            },
            required: ["user_id"],
          },
          {
            type: "object",
            properties: {
              recommendations: {
                type: "array",
                items: {
                  type: "object",
                  properties: {
                    product_id: { type: "string" },
                    score: { type: "number" },
                    reason: { type: "string" },
                    in_stock: { type: "boolean" },
                  },
                },
              },
              explanation: { type: "string" },
            },
          }
        ),
      },
    },
  },
  {
    id: "it-helpdesk-agent",
    title: "IT Helpdesk Agent",
    tags: ["helpdesk", "it", "chat", "support"],
    createdAt: "2024-05-15",
    description: "Resolves common IT issues and escalates when needed.",
    versions: {
      "1.0": {
        releaseDate: "2024-05-15",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("IT Helpdesk Agent", "1.0",
          {
            type: "object",
            properties: {
              issue: { type: "string" },
              system_info: { type: "object" },
            },
            required: ["issue"],
          },
          {
            type: "object",
            properties: {
              resolution: { type: "string" },
              ticket_id: { type: "string" },
              escalated: { type: "boolean" },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-07-20",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("IT Helpdesk Agent", "1.1",
          {
            type: "object",
            properties: {
              issue: { type: "string" },
              system_info: { type: "object" },
              auto_resolve: { type: "boolean" },
            },
            required: ["issue"],
          },
          {
            type: "object",
            properties: {
              resolution: { type: "string" },
              ticket_id: { type: "string" },
              escalated: { type: "boolean" },
              runbook_executed: { type: "boolean" },
            },
          }
        ),
      },
    },
  },
  {
    id: "financial-advisor-bot",
    title: "Financial Advisor Bot",
    tags: ["finance", "advisory", "analytics"],
    createdAt: "2024-06-01",
    description: "Provides general financial guidance and portfolio insights.",
    versions: {
      "1.0": {
        releaseDate: "2024-06-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Financial Advisor Bot", "1.0",
          {
            type: "object",
            properties: {
              query: { type: "string" },
              portfolio: { type: "object" },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              advice: { type: "string" },
              insights: { type: "array", items: { type: "string" } },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-08-01",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Financial Advisor Bot", "1.1",
          {
            type: "object",
            properties: {
              query: { type: "string" },
              portfolio: { type: "object" },
              risk_profile: { type: "string" },
            },
            required: ["query"],
          },
          {
            type: "object",
            properties: {
              advice: { type: "string" },
              insights: { type: "array", items: { type: "string" } },
              rebalancing_suggestions: { type: "array" },
              market_alerts: { type: "array", items: { type: "string" } },
            },
          }
        ),
      },
    },
  },
  {
    id: "content-moderation-agent",
    title: "Content Moderation Agent",
    tags: ["moderation", "safety", "classification"],
    createdAt: "2024-06-20",
    description: "Detects and flags policy-violating content automatically.",
    versions: {
      "1.0": {
        releaseDate: "2024-06-20",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Content Moderation Agent", "1.0",
          {
            type: "object",
            properties: {
              content: { type: "string" },
              content_type: { type: "string", description: "Type: text, image, or video." },
            },
            required: ["content"],
          },
          {
            type: "object",
            properties: {
              flagged: { type: "boolean" },
              violations: { type: "array", items: { type: "string" } },
              recommended_action: { type: "string" },
            },
          }
        ),
      },
      "1.1": {
        releaseDate: "2024-08-25",
        runtimeConfig: {
          model: { isSecrete: false, type: "string" },
          temperature: { isSecrete: false, type: 0.7 },
          apiKey: { isSecrete: true, type: "string" },
        },
        apiSpecs: buildSpec("Content Moderation Agent", "1.1",
          {
            type: "object",
            properties: {
              content: { type: "string" },
              content_type: { type: "string" },
              moderation_level: { type: "string" },
            },
            required: ["content"],
          },
          {
            type: "object",
            properties: {
              flagged: { type: "boolean" },
              violations: { type: "array", items: { type: "string" } },
              recommended_action: { type: "string" },
              confidence: { type: "number" },
              explanation: { type: "string" },
              appeal_id: { type: "string" },
            },
          }
        ),
      },
    },
  },
];
