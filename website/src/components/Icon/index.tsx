import React from 'react';
import * as LucideIcons from 'lucide-react';

interface IconProps {
  name: keyof typeof LucideIcons;
  size?: number;
  color?: string;
  className?: string;
  strokeWidth?: number;
}

/**
 * Reusable icon component using Lucide icons
 * 
 * @example
 * ```tsx
 * <Icon name="BookOpen" size={20} />
 * <Icon name="Rocket" color="#0088cc" />
 * ```
 */
export default function Icon({ 
  name, 
  size = 24, 
  color, 
  className = '',
  strokeWidth = 2 
}: IconProps) {
  const LucideIcon = LucideIcons[name];
  
  if (!LucideIcon) {
    console.warn(`Icon "${name}" not found in lucide-react`);
    return null;
  }
  
  return (
    <LucideIcon 
      size={size} 
      color={color} 
      className={className}
      strokeWidth={strokeWidth}
    />
  );
}
