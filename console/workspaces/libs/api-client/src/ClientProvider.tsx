import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const STALE_TIME = 10000;    
const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            refetchOnWindowFocus: false,
            staleTime: STALE_TIME,
        },
    },
});

export function ClientProvider({ children }: { children: React.ReactNode }) {

    return (
        <QueryClientProvider client={queryClient}>
            {children}
        </QueryClientProvider>
    );
}

export default ClientProvider;
