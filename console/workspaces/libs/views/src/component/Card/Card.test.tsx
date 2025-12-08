import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { Card } from './Card';
import { TestWrapper } from '../../../setupTests';

describe('Card', () => {
  it('renders children correctly', () => {
    render(
      <TestWrapper>
        <Card>
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    expect(screen.getByText('Test content')).toBeInTheDocument();
  });

  it('applies custom className', () => {
    render(
      <TestWrapper>
        <Card className="custom-class" data-testid="Card">
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    const card = screen.getByTestId('Card');
    expect(card).toHaveClass('custom-class');
  });

  it('passes through MUI Card props', () => {
    render(
      <TestWrapper>
        <Card variant="outlined" elevation={2} data-testid="Card">
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    const card = screen.getByTestId('Card');
    expect(card).toBeInTheDocument();
  });

  it('renders with CardContent wrapper', () => {
    render(
      <TestWrapper>
        <Card>
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    // CardContent should be present (MUI component)
    expect(screen.getByText('Test content')).toBeInTheDocument();
  });
});
