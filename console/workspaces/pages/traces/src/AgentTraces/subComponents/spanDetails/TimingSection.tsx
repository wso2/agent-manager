import { Span } from "@agent-management-platform/types";
import { InfoField } from "./InfoField";
import { InfoSection } from "./InfoSection";

interface TimingSectionProps {
    span: Span;
}

export function TimingSection({ span }: TimingSectionProps) {
    return (
        <InfoSection title="Timing">
            <InfoField
                label="Start Time"
                value={new Date(span.startTime).toLocaleString()}
            />
            <InfoField
                label="End Time"
                value={new Date(span.endTime).toLocaleString()}
            />
            <InfoField
                label="Duration"
                value={`${span.durationInNanos / 1000} ms`}
            />
        </InfoSection>
    );
}

