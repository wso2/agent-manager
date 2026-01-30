import { SearchHero } from "components/SearchHero";
import { motion, AnimatePresence } from "framer-motion";
import { useEffect, useRef, useState } from "react";
import { useAsgardeo } from "@asgardeo/react";
import { Plus, History, MessageSquare, PanelLeft, Globe, Trash2 } from "lucide-react";
import { Button } from "components/ui/button";
import { ChatHotelResults } from "components/ChatHotelResults";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
}

interface ChatSession {
  id: string;
  title: string;
  messages: Message[];
  sessionId: string;
}

type HotelResult = {
  hotelId: string;
  hotelName: string;
  city: string;
  country: string;
  rating: number;
  lowestPrice: number;
  amenities: string[];
  mapUrl: string;
  imageUrl: string;
};

type HotelResultsPayload = {
  type: "hotel_search";
  summary: string;
  currency: string;
  hotels: HotelResult[];
};

const HOTEL_RESULTS_MARKER = "HOTEL_RESULTS_JSON";

const stripCodeFence = (value: string) => {
  const trimmed = value.trim();
  if (trimmed.startsWith("```")) {
    const withoutFirst = trimmed.replace(/^```[a-zA-Z]*\n?/, "");
    return withoutFirst.replace(/```$/, "").trim();
  }
  return trimmed;
};

const tryParseHotelResults = (jsonText: string): HotelResultsPayload | null => {
  if (!jsonText) {
    return null;
  }
  try {
    const parsed = JSON.parse(jsonText) as HotelResultsPayload;
    if (parsed?.type !== "hotel_search" || !Array.isArray(parsed.hotels)) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
};

const parseHotelResults = (content: string): HotelResultsPayload | null => {
  const markerIndex = content.indexOf(HOTEL_RESULTS_MARKER);
  if (markerIndex !== -1) {
    const jsonText = stripCodeFence(
      content.slice(markerIndex + HOTEL_RESULTS_MARKER.length).trim()
    );
    const parsed = tryParseHotelResults(jsonText);
    if (parsed) {
      return parsed;
    }
  }

  const firstBrace = content.indexOf("{");
  const lastBrace = content.lastIndexOf("}");
  if (firstBrace !== -1 && lastBrace > firstBrace) {
    const candidate = stripCodeFence(content.slice(firstBrace, lastBrace + 1));
    return tryParseHotelResults(candidate);
  }

  return null;
};

const CHAT_API_URL = "http://localhost:9090/chat";
const CHAT_SESSIONS_URL = `${CHAT_API_URL}/sessions`;
const USER_ID_STORAGE_KEY = "travelPlannerUserId";
const SESSION_STORAGE_KEY = "travelPlannerSessions";
const ACTIVE_SESSION_STORAGE_KEY = "travelPlannerActiveSessionId";

const createSessionId = () =>
  `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;

const getOrCreateUserId = () => {
  if (typeof window === "undefined") {
    return "default";
  }
  const existing = localStorage.getItem(USER_ID_STORAGE_KEY);
  if (existing) {
    return existing;
  }
  const newId =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : createSessionId();
  localStorage.setItem(USER_ID_STORAGE_KEY, newId);
  return newId;
};

const buildWelcomeMessage = (): Message => ({
  id: createSessionId(),
  role: "assistant",
  content: "Hello! I'm your AI travel agent. Tell me about your dream trip, and I'll find the perfect places for you.",
});

const buildNewSession = (): ChatSession => {
  const sessionId = createSessionId();
  return {
    id: sessionId,
    title: "Current Session",
    sessionId,
    messages: [buildWelcomeMessage()],
  };
};

const loadStoredSessions = (): ChatSession[] | null => {
  if (typeof window === "undefined") {
    return null;
  }
  const raw = localStorage.getItem(SESSION_STORAGE_KEY);
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as ChatSession[]) : null;
  } catch {
    return null;
  }
};

const loadStoredActiveSessionId = (): string | null => {
  if (typeof window === "undefined") {
    return null;
  }
  return localStorage.getItem(ACTIVE_SESSION_STORAGE_KEY);
};

export default function Home() {
  const { isSignedIn, getAccessToken, user } = useAsgardeo();
  const [sessions, setSessions] = useState<ChatSession[]>(() => {
    const stored = loadStoredSessions();
    return stored && stored.length > 0 ? stored : [buildNewSession()];
  });
  const [activeSessionId, setActiveSessionId] = useState(() => {
    const stored = loadStoredActiveSessionId();
    const initial = stored || sessions[0]?.id;
    return initial ?? "";
  });
  const [userId, setUserId] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);
  const lastMessageRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(sessions));
  }, [sessions]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    if (activeSessionId) {
      localStorage.setItem(ACTIVE_SESSION_STORAGE_KEY, activeSessionId);
    }
  }, [activeSessionId]);

  useEffect(() => {
    let isMounted = true;
    const loadUserId = async () => {
      if (!isSignedIn) {
        return;
      }
      try {
        const resolved = user?.sub || user?.username || getOrCreateUserId();
        if (isMounted) {
          setUserId(resolved);
        }
      } catch {
        if (isMounted) {
          setUserId(getOrCreateUserId());
        }
      }
    };
    loadUserId();
    return () => {
      isMounted = false;
    };
  }, [isSignedIn, user]);

  useEffect(() => {
    if (!userId) {
      return;
    }
    const loadSessions = async () => {
      try {
        const token = isSignedIn ? await getAccessToken() : "";
        const headers: Record<string, string> = { Accept: "application/json" };
        if (token) {
          headers.Authorization = `Bearer ${token}`;
        }
        const response = await fetch(
          `${CHAT_SESSIONS_URL}?userId=${encodeURIComponent(userId)}`,
          { headers }
        );
        if (!response.ok) {
          return;
        }
        const data = await response.json();
        if (Array.isArray(data?.sessions) && data.sessions.length > 0) {
          setSessions(data.sessions);
          setActiveSessionId(data.sessions[0].id);
        }
      } catch (error) {
        if ((error as Error)?.name !== "AbortError") {
          console.error("Failed to load chat sessions", error);
        }
      }
    };
    loadSessions();
  }, [getAccessToken, isSignedIn, userId]);

  const activeSession = sessions.find(s => s.id === activeSessionId) || sessions[0];
  const messages = activeSession.messages;

  const handleNewChat = () => {
    const newSession = buildNewSession();
    newSession.title = `New Trip Planning ${sessions.length + 1}`;
    setSessions(prev => [newSession, ...prev]);
    setActiveSessionId(newSession.id);
  };

  const handleDeleteSession = (sessionId: string) => {
    setSessions(prev => {
      const next = prev.filter(session => session.id !== sessionId);
      const fallback = next.length > 0 ? next : [buildNewSession()];
      const nextActive =
        activeSessionId === sessionId ? (fallback[0]?.id ?? "") : activeSessionId;
      setActiveSessionId(nextActive);
      return fallback;
    });
  };

  const toggleSidebar = () => {
    setIsSidebarOpen(prev => !prev);
  };

  const handleSearch = async (query: string) => {
    const effectiveUserId = userId ?? getOrCreateUserId();
    if (!userId) {
      setUserId(effectiveUserId);
    }
    if (!activeSession) {
      return;
    }
    setIsLoading(true);
    const userMessage: Message = {
      id: Date.now().toString(),
      role: 'user',
      content: query
    };

    setSessions(prev => prev.map(s => 
      s.id === activeSessionId 
        ? { ...s, messages: [...s.messages, userMessage] }
        : s
    ));

    try {
      const token = isSignedIn ? await getAccessToken() : "";
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token) {
        headers.Authorization = `Bearer ${token}`;
      }
      const response = await fetch(CHAT_API_URL, {
        method: "POST",
        headers,
        body: JSON.stringify({
          message: query,
          sessionId: activeSession.sessionId,
        }),
      });

      if (!response.ok) {
        throw new Error(`Chat API error: ${response.status}`);
      }

      const data = await response.json();
      const assistantMessage: Message = {
        id: (Date.now() + 1).toString(),
        role: 'assistant',
        content: data?.message || "Sorry, I couldn't generate a response.",
      };

      setSessions(prev => prev.map(s =>
        s.id === activeSessionId
          ? {
              ...s,
              messages: [...s.messages, assistantMessage],
              title: s.title.startsWith('New Trip') ? query.slice(0, 20) : s.title
            }
          : s
      ));
    } catch (error) {
      const assistantMessage: Message = {
        id: (Date.now() + 1).toString(),
        role: 'assistant',
        content: "Sorry, I couldn't reach the travel assistant. Please try again.",
      };
      setSessions(prev => prev.map(s =>
        s.id === activeSessionId
          ? { ...s, messages: [...s.messages, assistantMessage] }
          : s
      ));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (!lastMessageRef.current) {
      return;
    }
    lastMessageRef.current.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [messages, isLoading]);

  return (
    <div className="min-h-screen flex bg-background tp-chat-shell">
      {/* Sidebar */}
      <aside
        className={`tp-chat-sidebar ${isSidebarOpen ? "tp-chat-sidebar--open" : "tp-chat-sidebar--closed"}`}
        aria-hidden={!isSidebarOpen}
      >
        <div className="p-4 border-b border-border">
          <Button onClick={handleNewChat} className="tp-chat-new-trip">
            <Plus className="w-4 h-4" />
            New Chat
          </Button>
        </div>
        
        <div className="tp-chat-session-list">
          <div className="tp-chat-session-heading">
            <History className="w-4 h-4" />
            Recent Plans
          </div>
          {sessions.map(s => (
            <button
              key={s.id}
              onClick={() => setActiveSessionId(s.id)}
              className={`tp-chat-session-button flex ${
                activeSessionId === s.id 
                ? 'tp-chat-session-button--active' 
                : 'tp-chat-session-button--idle'
              }`}
            >
              <MessageSquare className="w-5 h-5 shrink-0" />
              <span className="flex-1 min-w-0 truncate text-sm">{s.title}</span>

              <span className="tp-chat-session-spacer" />
              <span
                role="button"
                aria-label="Delete chat"
                className="tp-chat-session-delete"
                onClick={(event) => {
                  event.stopPropagation();
                  handleDeleteSession(s.id);
                }}
              >
                <Trash2 className="w-4 h-4" />
              </span>
            </button>
          ))}
        </div>
      </aside>

      {isSidebarOpen && (
        <button
          type="button"
          className="tp-chat-backdrop"
          onClick={toggleSidebar}
          aria-label="Close sessions sidebar"
        />
      )}

      <div className="flex-1 flex flex-col h-screen overflow-hidden tp-chat-main">
        <main className="flex-grow overflow-y-auto flex flex-col">
          <div className="w-full px-0 pt-0">
            <motion.div
              className="tp-chat-header"
              initial={{ opacity: 0, y: -8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.4 }}
            >
              <div className="tp-chat-header-left">
                <button
                  type="button"
                  onClick={toggleSidebar}
                  className="tp-chat-header-button"
                  aria-label="Toggle sessions sidebar"
                >
                  <PanelLeft className="w-4 h-4" />
                </button>
                <div className="tp-chat-title">
                  <span className="tp-chat-title-icon">
                    <Globe className="w-4 h-4" />
                  </span>
                  <span className="tp-chat-title-text">Hotel Booking Agent</span>
                </div>
              </div>
              <div className="tp-chat-header-actions" />
            </motion.div>
          </div>

          <div className="flex-grow space-y-8 mb-8 container mx-auto max-w-[56rem] px-4 pt-6 tp-chat-messages">
            <AnimatePresence mode="popLayout">
              {messages.map((msg, index) => {
                const hotelPayload =
                  msg.role === "assistant" ? parseHotelResults(msg.content) : null;
                const isLast = index === messages.length - 1 && !isLoading;
                return (
                <motion.div
                  key={msg.id}
                  ref={isLast ? lastMessageRef : undefined}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  className={`tp-chat-message ${msg.role === 'user' ? 'tp-chat-message--user' : 'tp-chat-message--assistant'}`}
                >
                  <div className="tp-chat-avatar-spacer" aria-hidden="true" />
                  
                  <div className={`space-y-4 max-w-[85%] ${msg.role === 'user' ? 'items-end' : ''}`}>
                    <div className={`tp-chat-bubble ${
                      msg.role === 'assistant' 
                        ? 'tp-chat-bubble--assistant' 
                        : 'tp-chat-bubble--user'
                    }`}>
                      {hotelPayload ? (
                        <ChatHotelResults payload={hotelPayload} />
                      ) : msg.role === "assistant" ? (
                        <div className="chat-markdown text-sm md:text-base leading-relaxed">
                          <ReactMarkdown
                            remarkPlugins={[remarkGfm]}
                            components={{
                              img: ({ ...props }) => (
                                <img className="chat-markdown-image" {...props} />
                              ),
                              a: ({ children, ...props }) => (
                                <a {...props} target="_blank" rel="noopener noreferrer">
                                  {children}
                                </a>
                              ),
                            }}
                          >
                            {msg.content}
                          </ReactMarkdown>
                        </div>
                      ) : (
                        <p className="text-sm md:text-base leading-relaxed whitespace-pre-line">{msg.content}</p>
                      )}
                    </div>

                  </div>
                </motion.div>
                );
              })}
              {isLoading && (
                <motion.div
                  ref={lastMessageRef}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  className="tp-chat-message tp-chat-message--assistant"
                >
                  <div className="tp-chat-avatar-spacer" aria-hidden="true" />
                  <div className="space-y-4 max-w-[85%]">
                    <div className="tp-chat-bubble tp-chat-bubble--assistant">
                      <div className="typing-dots" aria-label="Assistant is typing">
                        <span />
                        <span />
                        <span />
                      </div>
                    </div>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          <div className="container mx-auto max-w-[56rem] px-4 sticky bottom-4">
            <SearchHero onSearch={handleSearch} compact />
          </div>
        </main>
      </div>
    </div>
  );
}
