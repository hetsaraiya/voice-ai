import React from 'react';
import { render, screen, within } from '@testing-library/react';
import '@testing-library/jest-dom';

import { HomePage } from '@/app/pages/main/web-dashboard';

const mockGoToCreateAssistant = jest.fn();

jest.mock('@/hooks/use-credential', () => ({
  useCurrentCredential: () => ({ user: { id: 'user-1' } }),
}));

jest.mock('@/hooks/use-global-navigator', () => ({
  useGlobalNavigation: () => ({
    goToCreateAssistant: mockGoToCreateAssistant,
  }),
}));

jest.mock('@/app/components/carbon/button', () => ({
  PrimaryButton: ({ children, ...props }: any) => (
    <button {...props}>{children}</button>
  ),
}));

jest.mock('@carbon/react', () => ({
  ClickableTile: ({ children, href, className }: any) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
  Tag: ({ children, className }: any) => (
    <span className={className}>{children}</span>
  ),
  Button: ({ children, ...props }: any) => <button {...props}>{children}</button>,
  Link: ({ children, href }: any) => (
    <a href={href}>
      {children}
    </a>
  ),
}));

describe('Dashboard HomePage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('uses responsive quick-start grid classes', () => {
    const { container } = render(<HomePage />);

    const quickStartGrid = container.querySelector('section div.grid');
    expect(quickStartGrid).toBeInTheDocument();
    expect(quickStartGrid).toHaveClass('grid-cols-1');
    expect(quickStartGrid).toHaveClass('sm:grid-cols-2');
    expect(quickStartGrid).toHaveClass('xl:grid-cols-5');
  });

  it('does not force white text on all descendants of featured card', () => {
    render(<HomePage />);

    const featuredTile = screen.getByText('Build your first assistant').closest('a');
    expect(featuredTile).toBeInTheDocument();
    expect(featuredTile).not.toHaveClass('[&_*]:!text-white');
    expect(within(featuredTile as HTMLAnchorElement).getByText('Recommended')).toBeInTheDocument();
  });
});
