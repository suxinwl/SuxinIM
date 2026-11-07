import type { DefaultLabelFormatterCallbackParams } from 'echarts';

export interface ToolTipFormatterParams extends DefaultLabelFormatterCallbackParams {
  axisDim: string;
  axisIndex: number;
  axisType: string;
  axisId: string;
  axisValue: string;
  axisValueLabel: string;
}
