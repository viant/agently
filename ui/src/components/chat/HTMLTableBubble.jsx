import React from 'react';
import { Icon } from '@blueprintjs/core';
import { format as formatDate } from 'date-fns';

export default function HTMLTableBubble({message, context}) {
    const avatarColour = 'var(--blue4)';
    const bubbleClass = 'chat-bubble chat-user';

    return (
        <div className="chat-row user">
            <div style={{display:'flex', alignItems:'flex-start'}}>
                <div className="avatar" style={{background: avatarColour}}>
                    <Icon icon="person" color="var(--black)" size={12}/>
                </div>
                <div className={bubbleClass} data-ts={formatDate(new Date(message.createdAt), 'HH:mm')}>
                    {/* message.content already contains trusted HTML */}
                    <div
                        className="prose max-w-full text-sm"
                        style={{width:'60vw'}}
                        dangerouslySetInnerHTML={{__html: message.content}}
                    />
                </div>
            </div>
        </div>
    );
}
