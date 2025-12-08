import { FormProvider, UseFormReturn } from "react-hook-form";
import { Box } from "@wso2/oxygen-ui";
import { SourceAndConfiguration } from "./SourceAndConfiguration";
import { InputInterface } from "./InputInterface";
import { EnvironmentVariable } from "./EnvironmentVariable";
import { AddAgentFormValues } from "src/form/schema";

interface NewAgentFromSourceProps {
    methods: UseFormReturn<AddAgentFormValues>;
}

export const NewAgentFromSource = ({ methods }: NewAgentFromSourceProps) => {
    return (<FormProvider {...methods}>
        <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
          <SourceAndConfiguration />
          <InputInterface />
          <EnvironmentVariable />
        </Box>
      </FormProvider>)
}
