
import { FormProvider, UseFormReturn } from "react-hook-form";
import { AddAgentFormValues } from "src/form/schema";
import { ConnectAgentForm } from "./ConnectAgentForm";

interface ConnectNewAgentProps {
    methods: UseFormReturn<AddAgentFormValues>;
}

export const ConnectNewAgent = (props: ConnectNewAgentProps) => {

    const { methods } = props;
    
    return (
        <FormProvider {...methods}>
            <ConnectAgentForm />
        </FormProvider>
    );
};
