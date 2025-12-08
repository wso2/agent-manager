import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createAgent, deleteAgent, getAgent, listAgents } from "../apis";
import {
  AgentListResponse,
  AgentResponse,
  CreateAgentPathParams,
  CreateAgentRequest,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentsPathParams,
  ListAgentsQuery,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";

export function useListAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<AgentListResponse>({
    queryKey: ['agents', params, query],
    queryFn: () => listAgents(params, query, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useGetAgent(params: GetAgentPathParams) {
    const { getToken } = useAuthHooks();
    return useQuery<AgentResponse>({
        queryKey: ['agent', params],
        queryFn: () => getAgent(params, getToken),
        enabled: !!params.orgName && !!params.projName && !!params.agentName,
    });
}

export function useCreateAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    AgentResponse,
    unknown,
    { params: CreateAgentPathParams; body: CreateAgentRequest }
  >({
    mutationFn: ({ params, body }) => createAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

export function useDeleteAgent() {
    const { getToken } = useAuthHooks();
    const queryClient = useQueryClient();
    return useMutation<void, unknown, DeleteAgentPathParams>({
        mutationFn: (params) => deleteAgent(params, getToken),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['agents'] });
        },
    });
}
