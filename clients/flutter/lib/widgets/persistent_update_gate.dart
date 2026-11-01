import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

/// A confirmed mandatory update belongs above the Navigator. Replacing splash,
/// login or home routes must not dismiss it or make the business UI interactive.
class PersistentUpdateGate extends StatefulWidget {
  const PersistentUpdateGate(
      {super.key, required this.gate, required this.child});
  final ValueListenable<Widget?> gate;
  final Widget child;

  @override
  State<PersistentUpdateGate> createState() => _PersistentUpdateGateState();
}

class _PersistentUpdateGateState extends State<PersistentUpdateGate>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  Future<bool> didPopRoute() async => widget.gate.value != null;

  @override
  Widget build(BuildContext context) => ValueListenableBuilder<Widget?>(
        valueListenable: widget.gate,
        builder: (context, gate, _) => Stack(
          fit: StackFit.expand,
          children: [
            AbsorbPointer(
              absorbing: gate != null,
              child: ExcludeFocus(excluding: gate != null, child: widget.child),
            ),
            if (gate != null) ...[
              const ModalBarrier(dismissible: false, color: Colors.black54),
              Center(
                  child:
                      Material(type: MaterialType.transparency, child: gate)),
            ],
          ],
        ),
      );
}
