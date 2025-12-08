import { Span } from "@agent-management-platform/types";
import { InfoField } from "./InfoField";
import { InfoSection } from "./InfoSection";

interface BasicInfoSectionProps {
    span: Span;
}

export function BasicInfoSection({ span }: BasicInfoSectionProps) {
    return (
        <InfoSection title="Basic Information">
            <InfoField label="Span ID" value={span.spanId} isMonospace />
            <InfoField label="Trace ID" value={span.traceId} isMonospace />
            {span.parentSpanId && (
                <InfoField label="Parent Span ID" value={span.parentSpanId} isMonospace />
            )}
            <InfoField label="Name" value={span.name} />
            <InfoField label="Service" value={span.service} />
        </InfoSection>
    );
}

