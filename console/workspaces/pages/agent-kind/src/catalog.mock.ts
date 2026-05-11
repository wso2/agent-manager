export interface CatalogItemVersion {
  description: string;
  releaseDate: string;
  changes: string[];
  apiSpecs?: Object | null;
}

export interface CatalogItem {
  id: string;
  title: string;
  tags: string[];
  createdAt: string;
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

export const DUMMY_CATALOG_LIST: CatalogItem[] = [
  {
    id: "customer-support-agent",
    title: "Customer Support Agent",
    tags: ["chat", "rag", "customerSupport", "knowledgeBase"],
    createdAt: "2024-01-01",
    versions: {
      "1.0": {
        description: "Handles customer queries using RAG over a knowledge base.",
        releaseDate: "2024-01-01",
        changes: ["Initial release of the Customer Support Agent."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Customer's question or issue." },
              context: { type: "object", description: "Additional context for better answers." },
            },
            required: ["query"],
          },
          output: {
            type: "object",
            properties: {
              answer: { type: "string", description: "Agent's response to the customer's query." },
              sources: {
                type: "array",
                items: { type: "string" },
                description: "List of knowledge base entries used to generate the answer.",
              },
            },
          },
        },
      },
      "1.1": {
        description: "Enhanced support with sentiment analysis for better query handling.",
        releaseDate: "2024-02-15",
        changes: [
          "Integrated sentiment analysis to better understand customer emotions.",
          "Improved response generation based on detected sentiment.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Customer's question or issue." },
              context: { type: "object", description: "Additional context for better answers." },
            },
            required: ["query"],
          },
          output: {
            type: "object",
            properties: {
              answer: { type: "string", description: "Agent's response to the customer's query." },
              sources: {
                type: "array",
                items: { type: "string" },
                description: "List of knowledge base entries used to generate the answer.",
              },
              detected_sentiment: { type: "string", description: "Detected sentiment of the customer's query (e.g., positive, neutral, negative)." },
            },
          },
        },
      },
    },
  },
  {
    id: "document-retriever",
    title: "Document Retriever",
    tags: ["retriever", "vectorDB", "rag"],
    createdAt: "2024-01-15",
    versions: {
      "1.0": {
        description: "Retrieves and ranks relevant documents from a vector store.",
        releaseDate: "2024-01-15",
        changes: ["Initial release with vector similarity search."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Search query to retrieve relevant documents." },
              top_k: { type: "number", description: "Maximum number of documents to return. Defaults to 5." },
            },
            required: ["query"],
          },
          output: {
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
          },
        },
      },
      "1.1": {
        description: "Retrieves and re-ranks documents using hybrid search with BM25 and vector similarity.",
        releaseDate: "2024-03-10",
        changes: [
          "Added BM25 keyword search alongside vector search.",
          "Introduced re-ranking step for improved accuracy.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Search query to retrieve relevant documents." },
              top_k: { type: "number", description: "Maximum number of documents to return. Defaults to 5." },
              search_mode: { type: "string", description: "Search strategy to use: 'vector', 'bm25', or 'hybrid'. Defaults to 'hybrid'." },
            },
            required: ["query"],
          },
          output: {
            type: "object",
            properties: {
              documents: {
                type: "array",
                items: {
                  type: "object",
                  properties: {
                    id: { type: "string", description: "Document identifier." },
                    content: { type: "string", description: "Document content snippet." },
                    vector_score: { type: "number", description: "Vector similarity score." },
                    bm25_score: { type: "number", description: "BM25 keyword relevance score." },
                    final_score: { type: "number", description: "Combined re-ranking score." },
                  },
                },
                description: "Re-ranked list of retrieved documents.",
              },
            },
          },
        },
      },
    },
  },
  {
    id: "code-assistant",
    title: "Code Assistant",
    tags: ["code", "assistant", "developer"],
    createdAt: "2024-02-01",
    versions: {
      "1.0": {
        description: "Assists developers with code generation and reviews.",
        releaseDate: "2024-02-01",
        changes: ["Initial release with basic code generation support."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              prompt: { type: "string", description: "Code generation or review instruction." },
              language: { type: "string", description: "Target programming language (e.g., Python, JavaScript)." },
            },
            required: ["prompt"],
          },
          output: {
            type: "object",
            properties: {
              code: { type: "string", description: "Generated or reviewed code snippet." },
              explanation: { type: "string", description: "Explanation of the generated code or review feedback." },
            },
          },
        },
      },
      "1.1": {
        description: "Assists developers with multi-language code generation, inline reviews, and test scaffolding.",
        releaseDate: "2024-04-01",
        changes: [
          "Added support for Python, TypeScript, and Go.",
          "Introduced automated unit test scaffolding.",
          "Improved inline code review suggestions.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              prompt: { type: "string", description: "Code generation, review, or test scaffolding instruction." },
              language: { type: "string", description: "Target programming language (e.g., Python, TypeScript, Go)." },
              task: { type: "string", description: "Task type: 'generate', 'review', or 'test'. Defaults to 'generate'." },
            },
            required: ["prompt"],
          },
          output: {
            type: "object",
            properties: {
              code: { type: "string", description: "Generated or reviewed code snippet." },
              explanation: { type: "string", description: "Explanation of the generated code or review feedback." },
              tests: { type: "string", description: "Scaffolded unit tests (present when task is 'test')." },
            },
          },
        },
      },
    },
  },
  {
    id: "hr-policy-bot",
    title: "HR Policy Bot",
    tags: ["chat", "hr", "knowledgeBase"],
    createdAt: "2024-02-14",
    versions: {
      "1.0": {
        description: "Answers employee questions about HR policies and benefits.",
        releaseDate: "2024-02-14",
        changes: ["Initial release with HR policy Q&A."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              question: { type: "string", description: "Employee's HR policy question." },
            },
            required: ["question"],
          },
          output: {
            type: "object",
            properties: {
              answer: { type: "string", description: "Answer to the HR policy question." },
              policy_references: {
                type: "array",
                items: { type: "string" },
                description: "List of HR policy documents referenced.",
              },
            },
          },
        },
      },
      "1.1": {
        description: "Answers HR policy questions with role-based context and multi-region policy support.",
        releaseDate: "2024-04-20",
        changes: [
          "Added role-based policy filtering.",
          "Expanded knowledge base with multi-region HR policies.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              question: { type: "string", description: "Employee's HR policy question." },
              role: { type: "string", description: "Employee's role for role-based policy filtering (e.g., manager, engineer)." },
              region: { type: "string", description: "Employee's region for multi-region policy lookup (e.g., US, EU, APAC)." },
            },
            required: ["question"],
          },
          output: {
            type: "object",
            properties: {
              answer: { type: "string", description: "Answer to the HR policy question." },
              policy_references: {
                type: "array",
                items: { type: "string" },
                description: "List of HR policy documents referenced.",
              },
              applicable_regions: {
                type: "array",
                items: { type: "string" },
                description: "Regions to which the answer applies.",
              },
            },
          },
        },
      },
    },
  },
  {
    id: "sales-intelligence-agent",
    title: "Sales Intelligence Agent",
    tags: ["analytics", "sales", "insights"],
    createdAt: "2024-03-01",
    versions: {
      "1.0": {
        description: "Analyzes sales data and provides actionable insights.",
        releaseDate: "2024-03-01",
        changes: ["Initial release with basic sales analytics."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Natural language query about sales data." },
              time_range: { type: "object", description: "Optional date range with 'start' and 'end' fields (ISO 8601)." },
            },
            required: ["query"],
          },
          output: {
            type: "object",
            properties: {
              insights: {
                type: "array",
                items: { type: "string" },
                description: "List of actionable sales insights.",
              },
              summary: { type: "string", description: "High-level summary of the sales analysis." },
            },
          },
        },
      },
      "1.1": {
        description: "Delivers real-time sales insights with trend forecasting and competitor benchmarking.",
        releaseDate: "2024-05-05",
        changes: [
          "Added real-time data pipeline integration.",
          "Introduced trend forecasting using time-series models.",
          "Added competitor benchmarking module.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Natural language query about sales data." },
              time_range: { type: "object", description: "Optional date range with 'start' and 'end' fields (ISO 8601)." },
              include_forecast: { type: "boolean", description: "Whether to include trend forecasting in the response. Defaults to false." },
            },
            required: ["query"],
          },
          output: {
            type: "object",
            properties: {
              insights: {
                type: "array",
                items: { type: "string" },
                description: "List of actionable sales insights.",
              },
              summary: { type: "string", description: "High-level summary of the sales analysis." },
              forecast: { type: "object", description: "Time-series forecast data (present when include_forecast is true)." },
              competitor_benchmarks: {
                type: "array",
                items: { type: "object" },
                description: "Competitor performance benchmarks.",
              },
            },
          },
        },
      },
    },
  },
  {
    id: "legal-document-summarizer",
    title: "Legal Document Summarizer",
    tags: ["summarization", "legal", "rag"],
    createdAt: "2024-03-20",
    versions: {
      "1.0": {
        description: "Summarizes lengthy legal documents into concise briefs.",
        releaseDate: "2024-03-20",
        changes: ["Initial release with extractive summarization."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              document: { type: "string", description: "Full text of the legal document to summarize." },
              max_length: { type: "number", description: "Maximum length of the summary in words." },
            },
            required: ["document"],
          },
          output: {
            type: "object",
            properties: {
              summary: { type: "string", description: "Concise summary of the legal document." },
            },
          },
        },
      },
      "1.1": {
        description: "Generates structured legal briefs with clause extraction and risk flagging.",
        releaseDate: "2024-05-15",
        changes: [
          "Switched to abstractive summarization for better readability.",
          "Added clause-level extraction and categorization.",
          "Introduced risk-flag detection for common legal issues.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              document: { type: "string", description: "Full text of the legal document to summarize." },
              max_length: { type: "number", description: "Maximum length of the summary in words." },
              extract_clauses: { type: "boolean", description: "Whether to extract and categorize individual clauses. Defaults to false." },
            },
            required: ["document"],
          },
          output: {
            type: "object",
            properties: {
              summary: { type: "string", description: "Concise abstractive summary of the legal document." },
              clauses: {
                type: "array",
                items: { type: "object" },
                description: "Extracted and categorized clauses (present when extract_clauses is true).",
              },
              risk_flags: {
                type: "array",
                items: { type: "string" },
                description: "Detected risk indicators or problematic clauses.",
              },
            },
          },
        },
      },
    },
  },
  {
    id: "travel-booking-assistant",
    title: "Travel Booking Assistant",
    tags: ["chat", "travel", "booking"],
    createdAt: "2024-04-05",
    versions: {
      "1.0": {
        description: "Helps users plan and book travel itineraries.",
        releaseDate: "2024-04-05",
        changes: ["Initial release with flight and hotel search."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              destination: { type: "string", description: "Travel destination." },
              travel_dates: { type: "object", description: "Travel dates with 'departure' and 'return' fields (ISO 8601)." },
              preferences: { type: "object", description: "Optional traveler preferences (e.g., budget, cabin class)." },
            },
            required: ["destination", "travel_dates"],
          },
          output: {
            type: "object",
            properties: {
              itinerary: { type: "string", description: "Suggested travel itinerary." },
              flight_options: {
                type: "array",
                items: { type: "object" },
                description: "Available flight options.",
              },
              hotel_options: {
                type: "array",
                items: { type: "object" },
                description: "Available hotel options.",
              },
            },
          },
        },
      },
      "1.1": {
        description: "Plans end-to-end travel itineraries with real-time pricing, visa guidance, and local recommendations.",
        releaseDate: "2024-06-01",
        changes: [
          "Added real-time flight and hotel pricing via API.",
          "Integrated visa requirement lookup by nationality and destination.",
          "Added local activity and restaurant recommendations.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              destination: { type: "string", description: "Travel destination." },
              travel_dates: { type: "object", description: "Travel dates with 'departure' and 'return' fields (ISO 8601)." },
              preferences: { type: "object", description: "Optional traveler preferences (e.g., budget, cabin class)." },
              nationality: { type: "string", description: "Traveler's nationality for visa requirement lookup (ISO 3166-1 alpha-2)." },
            },
            required: ["destination", "travel_dates"],
          },
          output: {
            type: "object",
            properties: {
              itinerary: { type: "string", description: "Suggested end-to-end travel itinerary." },
              flight_options: {
                type: "array",
                items: { type: "object" },
                description: "Available flight options with real-time pricing.",
              },
              hotel_options: {
                type: "array",
                items: { type: "object" },
                description: "Available hotel options with real-time pricing.",
              },
              visa_requirements: { type: "object", description: "Visa requirements for the destination based on nationality." },
              local_recommendations: {
                type: "array",
                items: { type: "string" },
                description: "Recommended local activities and restaurants.",
              },
            },
          },
        },
      },
    },
  },
  {
    id: "medical-faq-agent",
    title: "Medical FAQ Agent",
    tags: ["chat", "medical", "knowledgeBase", "rag"],
    createdAt: "2024-04-18",
    versions: {
      "1.0": {
        description: "Answers frequently asked medical questions from verified sources.",
        releaseDate: "2024-04-18",
        changes: ["Initial release with curated medical FAQ knowledge base."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              question: { type: "string", description: "Medical question to answer." },
            },
            required: ["question"],
          },
          output: {
            type: "object",
            properties: {
              answer: { type: "string", description: "Evidence-based answer to the medical question." },
              disclaimer: { type: "string", description: "Medical disclaimer reminding users to consult a healthcare professional." },
            },
          },
        },
      },
      "1.1": {
        description: "Provides evidence-based medical answers with source citations and symptom triage guidance.",
        releaseDate: "2024-06-10",
        changes: [
          "Added inline source citations from verified medical databases.",
          "Introduced symptom triage flow for common conditions.",
          "Improved answer accuracy with updated knowledge base.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              question: { type: "string", description: "Medical question to answer." },
              symptoms: {
                type: "array",
                items: { type: "string" },
                description: "Optional list of symptoms for triage guidance.",
              },
            },
            required: ["question"],
          },
          output: {
            type: "object",
            properties: {
              answer: { type: "string", description: "Evidence-based answer to the medical question." },
              disclaimer: { type: "string", description: "Medical disclaimer reminding users to consult a healthcare professional." },
              sources: {
                type: "array",
                items: { type: "string" },
                description: "Citations from verified medical databases.",
              },
              triage_guidance: { type: "string", description: "Triage guidance based on provided symptoms (present when symptoms are given)." },
            },
          },
        },
      },
    },
  },
  {
    id: "ecommerce-product-advisor",
    title: "E-commerce Product Advisor",
    tags: ["recommendation", "ecommerce", "personalization"],
    createdAt: "2024-05-01",
    versions: {
      "1.0": {
        description: "Recommends products based on user preferences and history.",
        releaseDate: "2024-05-01",
        changes: ["Initial release with collaborative filtering recommendations."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              user_id: { type: "string", description: "Unique identifier of the user." },
              preferences: { type: "object", description: "Optional user preferences (e.g., categories, price range)." },
            },
            required: ["user_id"],
          },
          output: {
            type: "object",
            properties: {
              recommendations: {
                type: "array",
                items: {
                  type: "object",
                  properties: {
                    product_id: { type: "string", description: "Product identifier." },
                    score: { type: "number", description: "Recommendation confidence score." },
                    reason: { type: "string", description: "Reason for the recommendation." },
                  },
                },
                description: "List of recommended products.",
              },
            },
          },
        },
      },
      "1.1": {
        description: "Delivers hyper-personalized product recommendations using behavior signals, reviews, and inventory data.",
        releaseDate: "2024-07-01",
        changes: [
          "Combined collaborative and content-based filtering.",
          "Integrated real-time inventory and pricing signals.",
          "Added review sentiment analysis to boost recommendation quality.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              user_id: { type: "string", description: "Unique identifier of the user." },
              preferences: { type: "object", description: "Optional user preferences (e.g., categories, price range)." },
              context: { type: "object", description: "Optional session context (e.g., current page, cart contents)." },
            },
            required: ["user_id"],
          },
          output: {
            type: "object",
            properties: {
              recommendations: {
                type: "array",
                items: {
                  type: "object",
                  properties: {
                    product_id: { type: "string", description: "Product identifier." },
                    score: { type: "number", description: "Recommendation confidence score." },
                    reason: { type: "string", description: "Reason for the recommendation." },
                    in_stock: { type: "boolean", description: "Whether the product is currently in stock." },
                  },
                },
                description: "List of hyper-personalized recommended products.",
              },
              explanation: { type: "string", description: "Overall explanation of the recommendation strategy used." },
            },
          },
        },
      },
    },
  },
  {
    id: "it-helpdesk-agent",
    title: "IT Helpdesk Agent",
    tags: ["helpdesk", "it", "chat", "support"],
    createdAt: "2024-05-15",
    versions: {
      "1.0": {
        description: "Resolves common IT issues and escalates when needed.",
        releaseDate: "2024-05-15",
        changes: ["Initial release with ticket triage and FAQ resolution."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              issue: { type: "string", description: "Description of the IT issue." },
              system_info: { type: "object", description: "Optional system metadata (e.g., OS, application version)." },
            },
            required: ["issue"],
          },
          output: {
            type: "object",
            properties: {
              resolution: { type: "string", description: "Suggested resolution steps for the IT issue." },
              ticket_id: { type: "string", description: "Generated support ticket identifier." },
              escalated: { type: "boolean", description: "Whether the issue was escalated to a human agent." },
            },
          },
        },
      },
      "1.1": {
        description: "Resolves IT issues autonomously with runbook execution and smart escalation to on-call engineers.",
        releaseDate: "2024-07-20",
        changes: [
          "Added automated runbook execution for common fixes.",
          "Integrated on-call rotation for smart escalation.",
          "Improved ticket classification accuracy.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              issue: { type: "string", description: "Description of the IT issue." },
              system_info: { type: "object", description: "Optional system metadata (e.g., OS, application version)." },
              auto_resolve: { type: "boolean", description: "Whether to attempt automatic runbook execution. Defaults to true." },
            },
            required: ["issue"],
          },
          output: {
            type: "object",
            properties: {
              resolution: { type: "string", description: "Suggested or applied resolution for the IT issue." },
              ticket_id: { type: "string", description: "Generated support ticket identifier." },
              escalated: { type: "boolean", description: "Whether the issue was escalated to an on-call engineer." },
              runbook_executed: { type: "boolean", description: "Whether an automated runbook was executed to resolve the issue." },
            },
          },
        },
      },
    },
  },
  {
    id: "financial-advisor-bot",
    title: "Financial Advisor Bot",
    tags: ["finance", "advisory", "analytics"],
    createdAt: "2024-06-01",
    versions: {
      "1.0": {
        description: "Provides general financial guidance and portfolio insights.",
        releaseDate: "2024-06-01",
        changes: ["Initial release with basic portfolio analysis."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Financial question or topic to get guidance on." },
              portfolio: { type: "object", description: "Optional portfolio data for personalized insights." },
            },
            required: ["query"],
          },
          output: {
            type: "object",
            properties: {
              advice: { type: "string", description: "Financial guidance and recommendations." },
              insights: {
                type: "array",
                items: { type: "string" },
                description: "List of portfolio insights.",
              },
            },
          },
        },
      },
      "1.1": {
        description: "Delivers personalized financial planning with risk profiling, portfolio rebalancing suggestions, and market alerts.",
        releaseDate: "2024-08-01",
        changes: [
          "Added risk tolerance profiling questionnaire.",
          "Introduced portfolio rebalancing recommendations.",
          "Integrated real-time market alerts for held assets.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              query: { type: "string", description: "Financial question or planning topic." },
              portfolio: { type: "object", description: "Optional portfolio data for personalized planning." },
              risk_profile: { type: "string", description: "Risk tolerance level: 'conservative', 'moderate', or 'aggressive'." },
            },
            required: ["query"],
          },
          output: {
            type: "object",
            properties: {
              advice: { type: "string", description: "Personalized financial guidance and recommendations." },
              insights: {
                type: "array",
                items: { type: "string" },
                description: "List of portfolio insights.",
              },
              rebalancing_suggestions: {
                type: "array",
                items: { type: "object" },
                description: "Suggested portfolio rebalancing actions.",
              },
              market_alerts: {
                type: "array",
                items: { type: "string" },
                description: "Real-time market alerts for assets in the portfolio.",
              },
            },
          },
        },
      },
    },
  },
  {
    id: "content-moderation-agent",
    title: "Content Moderation Agent",
    tags: ["moderation", "safety", "classification"],
    createdAt: "2024-06-20",
    versions: {
      "1.0": {
        description: "Detects and flags policy-violating content automatically.",
        releaseDate: "2024-06-20",
        changes: ["Initial release with text classification for policy violations."],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              content: { type: "string", description: "Content to evaluate for policy violations." },
              content_type: { type: "string", description: "Type of content: 'text', 'image', or 'video'. Defaults to 'text'." },
            },
            required: ["content"],
          },
          output: {
            type: "object",
            properties: {
              flagged: { type: "boolean", description: "Whether the content violates policy." },
              violations: {
                type: "array",
                items: { type: "string" },
                description: "List of detected policy violation categories.",
              },
              recommended_action: { type: "string", description: "Recommended moderation action (e.g., remove, warn, allow)." },
            },
          },
        },
      },
      "1.1": {
        description: "Multi-modal content moderation with explainable decisions, appeal workflows, and confidence scoring.",
        releaseDate: "2024-08-25",
        changes: [
          "Added image and video moderation alongside text.",
          "Introduced confidence scores and explainable decision summaries.",
          "Added human-in-the-loop appeal workflow for borderline cases.",
        ],
        apiSpecs: {
          input: {
            type: "object",
            properties: {
              content: { type: "string", description: "Content to evaluate for policy violations." },
              content_type: { type: "string", description: "Type of content: 'text', 'image', or 'video'. Defaults to 'text'." },
              moderation_level: { type: "string", description: "Strictness of moderation: 'strict', 'standard', or 'lenient'. Defaults to 'standard'." },
            },
            required: ["content"],
          },
          output: {
            type: "object",
            properties: {
              flagged: { type: "boolean", description: "Whether the content violates policy." },
              violations: {
                type: "array",
                items: { type: "string" },
                description: "List of detected policy violation categories.",
              },
              recommended_action: { type: "string", description: "Recommended moderation action (e.g., remove, warn, allow)." },
              confidence: { type: "number", description: "Confidence score of the moderation decision (0-1)." },
              explanation: { type: "string", description: "Human-readable explanation of the moderation decision." },
              appeal_id: { type: "string", description: "Appeal case identifier for borderline decisions requiring human review." },
            },
          },
        },
      },
    },
  },
];

