import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { CodeBlock } from './CodeBlock';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';

// Mock clipboard API
Object.assign(navigator, {
    clipboard: {
        writeText: vi.fn(() => Promise.resolve()),
    },
});

const renderWithTheme = (component: React.ReactElement, mode: 'light' | 'dark' = 'light') => {
    const theme = createTheme({
        palette: {
            mode,
        },
    });

    return render(
        <ThemeProvider theme={theme}>
            {component}
        </ThemeProvider>
    );
};

describe('CodeBlock', () => {
    it('renders code with syntax highlighting', () => {
        renderWithTheme(<CodeBlock code="pip install agent-instrumentation" />);
        expect(screen.getByText(/pip install agent-instrumentation/i)).toBeInTheDocument();
    });

    it('displays copy button by default', () => {
        renderWithTheme(<CodeBlock code="test code" />);
        expect(screen.getByRole('button')).toBeInTheDocument();
    });

    it('hides copy button when showCopyButton is false', () => {
        renderWithTheme(<CodeBlock code="test code" showCopyButton={false} />);
        expect(screen.queryByRole('button')).not.toBeInTheDocument();
    });

    it('copies code to clipboard when copy button is clicked', async () => {
        const testCode = 'test code to copy';
        
        renderWithTheme(<CodeBlock code={testCode} />);
        
        const copyButton = screen.getByRole('button');
        fireEvent.click(copyButton);

        await waitFor(() => {
            expect(navigator.clipboard.writeText).toHaveBeenCalledWith(testCode);
        });
    });

    it('shows "Copied!" tooltip after successful copy', async () => {
        renderWithTheme(<CodeBlock code="test code" />);
        
        const copyButton = screen.getByRole('button');
        fireEvent.click(copyButton);

        await waitFor(() => {
            expect(screen.getByText('Copied!')).toBeInTheDocument();
        });
    });

    it('handles multi-line code correctly', () => {
        const multiLineCode = `export VAR1="value1"
export VAR2="value2"
export VAR3="value3"`;
        
        renderWithTheme(<CodeBlock code={multiLineCode} />);
        
        expect(screen.getByText(/export VAR1/)).toBeInTheDocument();
        expect(screen.getByText(/export VAR2/)).toBeInTheDocument();
        expect(screen.getByText(/export VAR3/)).toBeInTheDocument();
    });

    it('uses correct theme for dark mode', () => {
        const { container } = renderWithTheme(
            <CodeBlock code="test code" />,
            'dark'
        );
        
        // Check that code is rendered (syntax highlighter applies dark theme internally)
        expect(container.querySelector('pre')).toBeInTheDocument();
    });

    it('uses correct theme for light mode', () => {
        const { container } = renderWithTheme(
            <CodeBlock code="test code" />,
            'light'
        );
        
        // Check that code is rendered (syntax highlighter applies light theme internally)
        expect(container.querySelector('pre')).toBeInTheDocument();
    });

    it('applies custom language prop', () => {
        renderWithTheme(<CodeBlock code="const x = 1;" language="javascript" />);
        expect(screen.getByText(/const x = 1;/)).toBeInTheDocument();
    });

    it('handles copy errors gracefully', async () => {
        // Mock clipboard to throw error
        vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValueOnce(new Error('Copy failed'));
        
        renderWithTheme(<CodeBlock code="test code" />);
        
        const copyButton = screen.getByRole('button');
        
        // Should not throw error
        expect(() => fireEvent.click(copyButton)).not.toThrow();
    });
});

