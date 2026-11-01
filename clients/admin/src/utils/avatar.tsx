import { Avatar } from 'antd';

export const avatarList = [
  {
    src: new URL('@/assets/defualtAvatar/ic_avatar_01.png', import.meta.url).href,
    name: 'ic_avatar_01',
  },
  {
    src: new URL('@/assets/defualtAvatar/ic_avatar_02.png', import.meta.url).href,
    name: 'ic_avatar_02',
  },
  {
    src: new URL('@/assets/defualtAvatar/ic_avatar_03.png', import.meta.url).href,
    name: 'ic_avatar_03',
  },
  {
    src: new URL('@/assets/defualtAvatar/ic_avatar_04.png', import.meta.url).href,
    name: 'ic_avatar_04',
  },
  {
    src: new URL('@/assets/defualtAvatar/ic_avatar_05.png', import.meta.url).href,
    name: 'ic_avatar_05',
  },
  {
    src: new URL('@/assets/defualtAvatar/ic_avatar_06.png', import.meta.url).href,
    name: 'ic_avatar_06',
  },
];

export const defaultAvatarStr = (faceURL: string) => {
  for (const avatar of avatarList) {
    if (avatar.src === faceURL) {
      return avatar.name;
    }
  }
  return faceURL;
};

type DefaultAvatarProps = {
  faceURL: string;
  nickname: string;
};

export const DefaultAvatar = ({ faceURL, nickname }: DefaultAvatarProps) => {
  return faceURL.length ? (
    <Avatar
      shape="square"
      src={avatarList.find((avator) => avator.name === faceURL)?.src || faceURL}
    ></Avatar>
  ) : (
    <Avatar shape="square" style={{ backgroundColor: '#4096FF' }}>
      {nickname.split('')[0]}
    </Avatar>
  );
};
