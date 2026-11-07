declare module 'ot-json0' {
	import { Op } from '../types/ot';

	namespace OTJSON {
		export let type: {
			transform: (
				op: Op[],
				otherOp: Op[],
				type: 'left' | 'right',
			) => Op[];
			invert: (op: Op[]) => Op[];
			apply: (snapshot: any, op: Op[]) => any;
			canOpAffectPath: (op: Op, path: Path) => boolean;
		};
	}
	export default OTJSON;
}
