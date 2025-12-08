import { render, screen } from '@testing-library/react';
import { AddNewAgent } from './AddNewAgent';

describe('AddNewAgent', () => {
  it('renders without crashing', () => {
    render(<AddNewAgent />);
    expect(screen.getByText('Add New Agent')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Title';
    render(<AddNewAgent title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Description';
    render(<AddNewAgent description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });
});
