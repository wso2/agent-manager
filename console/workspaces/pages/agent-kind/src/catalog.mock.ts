export interface CatalogItemVersion {
  description: string;
  releaseDate: string;
  changes: string[];
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
      },
      "1.1": {
        description: "Enhanced support with sentiment analysis for better query handling.",
        releaseDate: "2024-02-15",
        changes: [
          "Integrated sentiment analysis to better understand customer emotions.",
          "Improved response generation based on detected sentiment.",
        ],
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
      },
      "1.1": {
        description: "Retrieves and re-ranks documents using hybrid search with BM25 and vector similarity.",
        releaseDate: "2024-03-10",
        changes: [
          "Added BM25 keyword search alongside vector search.",
          "Introduced re-ranking step for improved accuracy.",
        ],
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
      },
      "1.1": {
        description: "Assists developers with multi-language code generation, inline reviews, and test scaffolding.",
        releaseDate: "2024-04-01",
        changes: [
          "Added support for Python, TypeScript, and Go.",
          "Introduced automated unit test scaffolding.",
          "Improved inline code review suggestions.",
        ],
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
      },
      "1.1": {
        description: "Answers HR policy questions with role-based context and multi-region policy support.",
        releaseDate: "2024-04-20",
        changes: [
          "Added role-based policy filtering.",
          "Expanded knowledge base with multi-region HR policies.",
        ],
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
      },
      "1.1": {
        description: "Delivers real-time sales insights with trend forecasting and competitor benchmarking.",
        releaseDate: "2024-05-05",
        changes: [
          "Added real-time data pipeline integration.",
          "Introduced trend forecasting using time-series models.",
          "Added competitor benchmarking module.",
        ],
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
      },
      "1.1": {
        description: "Generates structured legal briefs with clause extraction and risk flagging.",
        releaseDate: "2024-05-15",
        changes: [
          "Switched to abstractive summarization for better readability.",
          "Added clause-level extraction and categorization.",
          "Introduced risk-flag detection for common legal issues.",
        ],
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
      },
      "1.1": {
        description: "Plans end-to-end travel itineraries with real-time pricing, visa guidance, and local recommendations.",
        releaseDate: "2024-06-01",
        changes: [
          "Added real-time flight and hotel pricing via API.",
          "Integrated visa requirement lookup by nationality and destination.",
          "Added local activity and restaurant recommendations.",
        ],
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
      },
      "1.1": {
        description: "Provides evidence-based medical answers with source citations and symptom triage guidance.",
        releaseDate: "2024-06-10",
        changes: [
          "Added inline source citations from verified medical databases.",
          "Introduced symptom triage flow for common conditions.",
          "Improved answer accuracy with updated knowledge base.",
        ],
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
      },
      "1.1": {
        description: "Delivers hyper-personalized product recommendations using behavior signals, reviews, and inventory data.",
        releaseDate: "2024-07-01",
        changes: [
          "Combined collaborative and content-based filtering.",
          "Integrated real-time inventory and pricing signals.",
          "Added review sentiment analysis to boost recommendation quality.",
        ],
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
      },
      "1.1": {
        description: "Resolves IT issues autonomously with runbook execution and smart escalation to on-call engineers.",
        releaseDate: "2024-07-20",
        changes: [
          "Added automated runbook execution for common fixes.",
          "Integrated on-call rotation for smart escalation.",
          "Improved ticket classification accuracy.",
        ],
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
      },
      "1.1": {
        description: "Delivers personalized financial planning with risk profiling, portfolio rebalancing suggestions, and market alerts.",
        releaseDate: "2024-08-01",
        changes: [
          "Added risk tolerance profiling questionnaire.",
          "Introduced portfolio rebalancing recommendations.",
          "Integrated real-time market alerts for held assets.",
        ],
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
      },
      "1.1": {
        description: "Multi-modal content moderation with explainable decisions, appeal workflows, and confidence scoring.",
        releaseDate: "2024-08-25",
        changes: [
          "Added image and video moderation alongside text.",
          "Introduced confidence scores and explainable decision summaries.",
          "Added human-in-the-loop appeal workflow for borderline cases.",
        ],
      },
    },
  },
];

