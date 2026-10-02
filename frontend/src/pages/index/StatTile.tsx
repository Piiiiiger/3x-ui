import type { ReactNode } from 'react';
import { Card } from 'antd';

interface StatTileProps {
  icon: ReactNode;
  label: string;
  value: ReactNode;
  unit?: string;
  detail: ReactNode;
  children?: ReactNode;
}

export default function StatTile({ icon, label, value, unit, detail, children }: StatTileProps) {
  return (
    <Card hoverable className="ov-tile" styles={{ body: { padding: 0 } }}>
      <div className="ov-tile-head">
        <span className="ov-tile-icon">{icon}</span>
        <span className="ov-kicker ov-card-title">{label}</span>
      </div>
      <div className="ov-tile-value">
        <span className="ov-tile-number">{value}</span>
        {unit && <span className="ov-tile-unit">{unit}</span>}
      </div>
      <div className="ov-tile-detail">{detail}</div>
      {children && <div className="ov-tile-extra">{children}</div>}
    </Card>
  );
}
