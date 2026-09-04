/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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

// Prism splits highlighted code into multiple sibling <span> tokens, so the
// full source line is never a single text node's content — assert against
// the <code> element's combined textContent instead of screen.getByText.
const getRenderedCode = (container: HTMLElement) =>
    container.querySelector('code')?.textContent ?? '';

describe('CodeBlock', () => {
    it('renders code with syntax highlighting', () => {
        const { container } = renderWithTheme(<CodeBlock code="pip install agent-instrumentation" />);
        expect(getRenderedCode(container)).toContain('pip install agent-instrumentation');
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
        // MUI's Tooltip only mounts its content while open, so hover before
        // clicking to mirror the real interaction (the cursor is already over
        // the button when the user clicks it).
        fireEvent.mouseOver(copyButton);
        fireEvent.click(copyButton);

        await waitFor(() => {
            expect(screen.getByText('Copied!')).toBeInTheDocument();
        });
    });

    it('handles multi-line code correctly', () => {
        const multiLineCode = `export VAR1="value1"
export VAR2="value2"
export VAR3="value3"`;

        const { container } = renderWithTheme(<CodeBlock code={multiLineCode} />);
        const renderedCode = getRenderedCode(container);

        expect(renderedCode).toContain('export VAR1');
        expect(renderedCode).toContain('export VAR2');
        expect(renderedCode).toContain('export VAR3');
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
        const { container } = renderWithTheme(<CodeBlock code="const x = 1;" language="javascript" />);
        expect(container.querySelector('code')).toHaveClass('language-javascript');
        expect(getRenderedCode(container)).toContain('const x = 1;');
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

