import { Box, Flex, FlexProps, HStack, Text, VStack } from '@chakra-ui/react';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

const NsListItem = ({
  isSelected,
  isPrivate,
  displayPoint = false,
  teamName,
  ...flexprop
}: {
  displayPoint: boolean;
  teamName: string;
  isPrivate: boolean;
  isSelected: boolean;
} & FlexProps) => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  return (
    <Flex
      align={'center'}
      p="6px 4px"
      mb="2px"
      position={'relative'}
      borderRadius="4px"
      onClick={(e) => {
        e.preventDefault();
        queryClient.invalidateQueries({ queryKey: ['teamList'] });
      }}
      cursor={'pointer'}
      {...flexprop}
      {...(isSelected
        ? {
            background: 'rgba(0, 0, 0, 0.05)'
          }
        : {
            bgColor: 'unset'
          })}
      _hover={{
        '> .namespace-option': {
          display: 'flex'
        },
        bgColor: 'rgba(0, 0, 0, 0.03)'
      }}
    >
      <HStack gap={'8px'} align={'center'} width={'full'}>
        <Box
          h="8px"
          w={displayPoint ? '8px' : '0'}
          m="4px"
          borderRadius="50%"
          bgColor={isSelected ? '#47C8BF' : '#9699B4'}
        />
        <Text
          {...(isSelected
            ? {
                color: '#0884DD'
              }
            : {})}
          textTransform={'capitalize'}
        >
          {isPrivate ? t('Default Team') : teamName}
        </Text>
      </HStack>
    </Flex>
  );
};

export default NsListItem;
