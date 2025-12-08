import React from 'react';
import { Card as MuiCard, CardContent, CardProps as MuiCardProps } from '@wso2/oxygen-ui';
import clsx from 'clsx';

interface CardProps extends Omit<MuiCardProps, 'children'> {
  children: React.ReactNode;
  className?: string;
}

export function Card({ children, className, ...muiProps }: CardProps) {
  return (
    <MuiCard
      data-testid="Card"
      className={clsx(className)}
      {...muiProps}
    >
      <CardContent>
        {children}
      </CardContent>
    </MuiCard>
  );
}
