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

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ThemeProvider } from '@wso2/oxygen-ui';
import { CssBaseline } from '@wso2/oxygen-ui';
import { describe, it, expect, vi } from 'vitest';
import { MainLayout } from './MainLayout';
import { aiAgentTheme } from '../../theme';

const renderWithTheme = (component: React.ReactElement) => {
  return render(
    <ThemeProvider theme={aiAgentTheme}>
      <CssBaseline />
      {component}
    </ThemeProvider>
  );
};

describe('MainLayout', () => {
  it('renders with default props', () => {
    renderWithTheme(<MainLayout />);
    
    expect(screen.getByText('AI Agent Manager')).toBeInTheDocument();
  });

  it('renders user avatar when user is provided', () => {
    const user = {
      name: 'John Doe',
      email: 'john@example.com',
    };
    
    renderWithTheme(<MainLayout user={user} />);
    
    expect(screen.getByLabelText('account of current user')).toBeInTheDocument();
  });

  it('renders user avatar with image when provided', () => {
    const user = {
      name: 'John Doe',
      email: 'john@example.com',
      avatar: 'https://example.com/avatar.jpg',
    };
    
    renderWithTheme(<MainLayout user={user} />);
    
    const avatar = screen.getByAltText('John Doe');
    expect(avatar).toBeInTheDocument();
    expect(avatar).toHaveAttribute('src', 'https://example.com/avatar.jpg');
  });

  it('opens user menu when avatar is clicked', async () => {
    const user = {
      name: 'John Doe',
      email: 'john@example.com',
    };
    
    renderWithTheme(<MainLayout user={user} />);
    
    const avatarButton = screen.getByLabelText('account of current user');
    fireEvent.click(avatarButton);
    
    await waitFor(() => {
      expect(screen.getByText('John Doe')).toBeInTheDocument();
      expect(screen.getByText('john@example.com')).toBeInTheDocument();
    });
  });

  it('calls onLogout when logout is clicked', async () => {
    const onLogout = vi.fn();
    const user = {
      name: 'John Doe',
      email: 'john@example.com',
    };
    
    renderWithTheme(<MainLayout user={user} onLogout={onLogout} />);
    
    const avatarButton = screen.getByLabelText('account of current user');
    fireEvent.click(avatarButton);
    
    await waitFor(() => {
      const logoutButton = screen.getByText('Logout');
      fireEvent.click(logoutButton);
    });
    
    expect(onLogout).toHaveBeenCalled();
  });

  it('renders custom navigation items in mobile drawer', async () => {
    const navigationItems = [
      { label: 'Home', href: '/home' },
      { label: 'About', href: '/about' },
    ];
    
    // Mock mobile breakpoint
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 600,
    });
    
    renderWithTheme(<MainLayout navigationItems={navigationItems} />);
    
    const menuButton = screen.getByLabelText('toggle sidebar');
    fireEvent.click(menuButton);
    
    await waitFor(() => {
      expect(screen.getByText('Home')).toBeInTheDocument();
      expect(screen.getByText('About')).toBeInTheDocument();
    });
  });

  it('renders left elements when provided', () => {
    const leftElements = <button>Left Button</button>;
    
    renderWithTheme(<MainLayout leftElements={leftElements} />);
    
    expect(screen.getByText('Left Button')).toBeInTheDocument();
  });

  it('renders right elements when provided', () => {
    const rightElements = <button>Right Button</button>;
    
    renderWithTheme(<MainLayout rightElements={rightElements} />);
    
    expect(screen.getByText('Right Button')).toBeInTheDocument();
  });

  it('closes mobile drawer when navigation item is clicked', async () => {
    const navigationItems = [
      { label: 'Home', href: '/home' },
    ];
    
    // Mock mobile breakpoint
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 600,
    });
    
    renderWithTheme(<MainLayout navigationItems={navigationItems} />);
    
    const menuButton = screen.getByLabelText('toggle sidebar');
    fireEvent.click(menuButton);
    
    await waitFor(() => {
      const homeButton = screen.getByText('Home');
      fireEvent.click(homeButton);
    });
    
    // Drawer should be closed
    await waitFor(() => {
      expect(screen.queryByText('Home')).not.toBeInTheDocument();
    });
  });

  it('renders custom user menu items', async () => {
    const userMenuItems = [
      { label: 'Custom Item', onClick: vi.fn(async () => {}) },
    ];
    const user = {
      name: 'John Doe',
      email: 'john@example.com',
    };
    
    renderWithTheme(<MainLayout user={user} userMenuItems={userMenuItems} />);
    
    const avatarButton = screen.getByLabelText('account of current user');
    fireEvent.click(avatarButton);
    
    await waitFor(() => {
      expect(screen.getByText('Custom Item')).toBeInTheDocument();
    });
  });

  it('renders navigation items on desktop sidebar', () => {
    // Mock desktop breakpoint
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 1200,
    });
    
    const navigationItems = [
      { label: 'Home', href: '/home' },
      { label: 'About', href: '/about' },
    ];
    
    renderWithTheme(<MainLayout navigationItems={navigationItems} />);
    
    expect(screen.getByText('Home')).toBeInTheDocument();
    expect(screen.getByText('About')).toBeInTheDocument();
  });

  it('calls onClick when desktop navigation item is clicked', () => {
    // Mock desktop breakpoint
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 1200,
    });
    
    const onClick = vi.fn();
    const navigationItems = [
      { label: 'Home', onClick },
    ];
    
    renderWithTheme(<MainLayout navigationItems={navigationItems} />);
    
    const homeButton = screen.getByText('Home');
    fireEvent.click(homeButton);
    expect(onClick).toHaveBeenCalled();
  });

  it('renders children content', () => {
    const children = <div data-testid="main-content">Main Content</div>;
    
    renderWithTheme(<MainLayout>{children}</MainLayout>);
    
    expect(screen.getByTestId('main-content')).toBeInTheDocument();
    expect(screen.getByText('Main Content')).toBeInTheDocument();
  });

  it('calls onSidebarToggle when sidebar toggle button is clicked', () => {
    const onSidebarToggle = vi.fn();
    
    renderWithTheme(<MainLayout onSidebarToggle={onSidebarToggle} />);
    
    const toggleButton = screen.getByLabelText('toggle sidebar');
    fireEvent.click(toggleButton);
    
    expect(onSidebarToggle).toHaveBeenCalledWith(true); // Should be called with collapsed state
  });

  it('renders with collapsed sidebar when sidebarCollapsed is true', () => {
    renderWithTheme(<MainLayout sidebarCollapsed={true} />);
    
    // The sidebar should be collapsed (we can't easily test the visual state, 
    // but we can test that the component renders without errors)
    expect(screen.getByLabelText('toggle sidebar')).toBeInTheDocument();
  });

  it('renders default navigation items when none provided', () => {
    renderWithTheme(<MainLayout />);
    
    // Should render default navigation items
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Agents')).toBeInTheDocument();
    expect(screen.getByText('Analytics')).toBeInTheDocument();
  });

  it('renders default user menu items when none provided', async () => {
    const user = {
      name: 'John Doe',
      email: 'john@example.com',
    };
    
    renderWithTheme(<MainLayout user={user} />);
    
    const avatarButton = screen.getByLabelText('account of current user');
    fireEvent.click(avatarButton);
    
    await waitFor(() => {
      expect(screen.getByText('Profile')).toBeInTheDocument();
      expect(screen.getByText('Settings')).toBeInTheDocument();
      expect(screen.getByText('Notifications')).toBeInTheDocument();
      expect(screen.getByText('Logout')).toBeInTheDocument();
    });
  });
});
