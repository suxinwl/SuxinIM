import 'dart:ui';

Rect protectedCropRect(Size image,double requestedAspect,double zoom,Offset normalizedCenter){
 if(image.width<=0||image.height<=0||!image.width.isFinite||!image.height.isFinite||!zoom.isFinite||zoom<1||zoom>4||!requestedAspect.isFinite||requestedAspect<0||!normalizedCenter.dx.isFinite||!normalizedCenter.dy.isFinite)throw ArgumentError('Invalid crop geometry');
 final aspect=requestedAspect==0?image.width/image.height:requestedAspect;
 var width=image.width/zoom,height=width/aspect;
 if(height>image.height/zoom){height=image.height/zoom;width=height*aspect;}
 final left=(normalizedCenter.dx*image.width-width/2).clamp(0,image.width-width).toDouble();
 final top=(normalizedCenter.dy*image.height-height/2).clamp(0,image.height-height).toDouble();
 return Rect.fromLTWH(left,top,width,height);
}
