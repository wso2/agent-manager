"use client";

import { useRef, useState } from "react";
import React from "react";
import ChatComposer from "./ChatComposer";
import ChatEmptyState from "./ChatEmptyState";
import TopBar from "./TopBar";
import { cancelJob, sendMessage } from "@/lib/api";

type ReportStatus = "queued" | "running" | "ready" | "failed" | "cancelled";

type ReportItem = {
  id: string;
  request: string;
  status: ReportStatus;
  report?: string;
  error?: string;
  createdAt: string;
  progress: string[];
};

const STATUS_LABELS: Record<ReportStatus, string> = {
  queued: "Queued",
  running: "Running",
  ready: "Ready",
  failed: "Failed",
  cancelled: "Cancelled",
};

const formatMessageContent = (content: string) => {
  const hasLimitations = content.includes("Limitations:");
  const hasSources = content.includes("Sources:") || content.includes("sources:");

  if (hasLimitations || hasSources) {
    let mainContent = content;
    let additionalDetails = "";

    const limitationsMatch = content.match(
      /(Limitations:|Sources:|References:|Note:|Disclaimer:)[\s\S]*/i,
    );
    if (limitationsMatch) {
      mainContent = content.substring(0, limitationsMatch.index).trim();
      additionalDetails = limitationsMatch[0].trim();
    }

    return { mainContent, additionalDetails, hasDetails: !!additionalDetails };
  }

  return { mainContent: content, additionalDetails: "", hasDetails: false };
};

function MessageContent({ content }: { content: string }) {
  const [showDetails, setShowDetails] = React.useState(false);
  const formatted = formatMessageContent(content);

  return (
    <>
      <div className="message-main-content">{formatted.mainContent}</div>
      {formatted.hasDetails ? (
        <div className="message-details">
          <button
            type="button"
            onClick={() => setShowDetails(!showDetails)}
            className="message-details-toggle"
          >
            {showDetails ? "Hide details" : "More info"}
          </button>
          {showDetails ? (
            <div className="message-details-content">
              {formatted.additionalDetails}
            </div>
          ) : null}
        </div>
      ) : null}
    </>
  );
}

const formatTime = (value?: string) => {
  if (!value) {
    return "";
  }

  try {
    return new Date(value).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return "";
  }
};

const createReportItem = (request: string): ReportItem => ({
  id: `local-${Date.now()}-${Math.random().toString(16).slice(2)}`,
  request,
  status: "queued",
  report: "",
  error: "",
  createdAt: new Date().toISOString(),
  progress: [],
});

export default function ChatView() {
  const [report, setReport] = useState<ReportItem | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");
  const abortRef = useRef<AbortController | null>(null);
  const jobIdRef = useRef<string | null>(null);
  const updateReport = (updater: (current: ReportItem) => ReportItem) => {
    setReport((current) => (current ? updater(current) : current));
  };

  const notifyReportReady = (requestText: string) => {
    if (typeof window === "undefined" || !("Notification" in window)) {
      return;
    }

    const title = "Finance Insight";
    const body = `Report ready: ${requestText}`;
    if (Notification.permission === "granted") {
      new Notification(title, { body, tag: "finance-insight-report" });
      return;
    }

    if (Notification.permission !== "denied") {
      Notification.requestPermission().then((permission) => {
        if (permission === "granted") {
          new Notification(title, { body, tag: "finance-insight-report" });
        }
      });
    }
  };

  const cancelInFlight = () => {
    if (!isLoading) {
      return;
    }
    try {
      abortRef.current?.abort();
    } catch {
      // Ignore abort errors from browsers that throw on aborted signals.
    }
    if (jobIdRef.current) {
      void cancelJob(jobIdRef.current);
    }
    updateReport((current) => ({
      ...current,
      status: "cancelled",
      progress: current.progress.slice(-5),
    }));
    setIsLoading(false);
    setError("");
  };

  const handleNewRequest = () => {
    cancelInFlight();
    setReport(null);
    setError("");
  };

  const handleSend = async (content: string) => {
    if (!content.trim() || isLoading) {
      return;
    }

    setError("");
    setIsLoading(true);

    const nextReport = createReportItem(content);
    setReport(nextReport);

    const abortController = new AbortController();
    abortRef.current = abortController;
    jobIdRef.current = null;

    try {
      const response = await sendMessage(content, {
        onTrace: (message: string) => {
          updateReport((item) => ({
            ...item,
            status: "running",
            progress: [...item.progress, message].slice(-6),
          }));
        },
        onJobId: (jobId) => {
          jobIdRef.current = jobId;
          updateReport((item) => ({
            ...item,
            id: jobId,
            status: "running",
          }));
        },
        signal: abortController.signal,
      });

      updateReport((item) => ({
        ...item,
        status: "ready",
        report: response.report || "",
        progress: item.progress.slice(-6),
      }));
      notifyReportReady(content);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      const errorMsg = err instanceof Error ? err.message : "Request failed";
      updateReport((item) => ({
        ...item,
        status: "failed",
        error: errorMsg,
        progress: item.progress.slice(-6),
      }));
      if (errorMsg.includes("Cannot connect")) {
        setError(
          "Cannot connect to API server. Please check if the backend is running.",
        );
      } else if (errorMsg.toLowerCase().includes("cancelled")) {
        setError("");
      } else {
        setError(errorMsg);
      }
    } finally {
      setIsLoading(false);
      abortRef.current = null;
      jobIdRef.current = null;
    }
  };

  const hasReport = Boolean(report?.report?.trim());

  return (
    <main className="chat-main">
      <TopBar onNewChat={handleNewRequest} />
      <section className="chat-content">
        <div className="chat-view">
          {error ? <div className="banner banner-error">{error}</div> : null}
          {report ? (
            <div className="report-panel">
              <div
                className={`report-panel-card ${
                  report.status === "ready" ? "report-panel-card--ready" : ""
                } ${
                  report.status === "running" ? "report-panel-card--running" : ""
                }`}
              >
                <div className="report-panel-header">
                  <div>
                    <div className="report-panel-title">Report</div>
                    <div className="report-panel-request">{report.request}</div>
                  </div>
                  <div className="report-panel-meta">
                    <span className={`report-status report-status--${report.status}`}>
                      {STATUS_LABELS[report.status]}
                    </span>
                    {report.status === "ready" ? (
                      <span className="report-ready-dot" />
                    ) : null}
                    <span className="report-time">
                      {formatTime(report.createdAt)}
                    </span>
                  </div>
                </div>

                {report.error ? (
                  <div className="banner banner-error">{report.error}</div>
                ) : null}

                {report.status === "ready" ? (
                  hasReport ? (
                    <div className="report-output">
                      <div className="report-output-title">Report</div>
                      <MessageContent content={report.report || ""} />
                    </div>
                  ) : (
                    <div className="banner banner-error">
                      No report returned. Check backend logs for details.
                    </div>
                  )
                ) : report.status === "cancelled" ? (
                  <div className="report-placeholder">Request cancelled.</div>
                ) : (
                  <div className="report-activity">
                    <div className="report-activity-title">Activity</div>
                    {report.progress.length ? (
                      report.progress.slice(-5).map((item, index) => (
                        <div className="report-activity-item" key={index}>
                          {item}
                        </div>
                      ))
                    ) : (
                      <div className="report-activity-empty">
                        Working on it now...
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          ) : (
            <ChatEmptyState onSelectScenario={handleSend} />
          )}
          {!report ? (
            <ChatComposer
              disabled={isLoading}
              loading={isLoading}
              onSend={handleSend}
              onStop={cancelInFlight}
            />
          ) : null}
        </div>
      </section>
    </main>
  );
}
